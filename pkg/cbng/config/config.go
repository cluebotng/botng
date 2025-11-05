package config

import (
	"github.com/cluebotng/botng/pkg/cbng/wikipedia"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"os"
	"sync"
)

var ReleaseTag = "development"
var RecentRevertThreshold = int64(86400)
var RecentChangeWindow = int64(14 * 86400)

type BotConfiguration struct {
	Owner    string
	Friends  []string
	Run      bool
	Angry    bool
	ReadOnly bool
}

type WikipediaConfiguration struct {
	Username string
	Password string
	Host     string
}

type SqlConfiguration struct {
	Username string
	Password string
	Host     string
	Port     int
	Schema   string
}

type SqlInstanceConfiguration struct {
	Replica []SqlConfiguration
	Cluebot SqlConfiguration
}

type DynamicConfiguration struct {
	HuggleUserWhitelist []string
	TFA                 string
	AngryOptinPages     []string
	NamespaceOptIn      []string
	Run                 bool
}

type IrcRelayChannelConfiguration struct {
	Spam   string
	Revert string
	Debug  string
}

type IrcConfiguration struct {
	Server   string
	Port     int
	Username string
	Password string
	Channel  IrcRelayChannelConfiguration
}

type CoreConfiguration struct {
	Host string
	Port int
}

type HoneyConfiguration struct {
	Key        string
	SampleRate float64
}

type LoggingConfiguration struct {
	File string
	Keep int
}

type Instances struct {
	AngryOptInConfiguration *AngryOptInConfigurationInstance
	HuggleConfiguration     *HuggleConfigurationInstance
	NamespaceOptIn          *NamespaceOptInInstance
	Run                     *RunInstance
	TFA                     *TFAInstance
}

type Configuration struct {
	Runtime struct {
		Release string
	}
	Bot       BotConfiguration
	Wikipedia WikipediaConfiguration
	Sql       SqlInstanceConfiguration
	Dynamic   DynamicConfiguration
	Instances Instances
	Irc       IrcConfiguration
	Core      CoreConfiguration
	Honey     HoneyConfiguration
	Logging   LoggingConfiguration
}

func envVarWithDefault(key, fallback string) string {
	value := os.Getenv(key)
	if len(value) == 0 {
		return fallback
	}
	return value
}

func NewConfiguration() *Configuration {
	logger := logrus.WithField("function", "config.NewConfiguration")

	configuration := Configuration{
		Runtime: struct{ Release string }{Release: ReleaseTag},
		Bot: BotConfiguration{
			Owner: "NaomiAmethyst",
			Friends: []string{
				"ClueBot",
				"DASHBotAV",
			},
			Run:      envVarWithDefault("CBNG_CFG_RUN", "true") == "true",
			Angry:    envVarWithDefault("CBNG_CFG_ANGRY", "false") == "true",
			ReadOnly: envVarWithDefault("CBNG_CFG_READ_ONLY", "true") == "true",
		},
		Wikipedia: WikipediaConfiguration{
			Host:     "en.wikipedia.org",
			Username: "ClueBot_NG",
			Password: envVarWithDefault("CBNG_WIKIPEDIA_PASSWORD", ""),
		},
		Irc: IrcConfiguration{
			Server:   "irc.libera.chat",
			Port:     6697,
			Username: envVarWithDefault("CBNG_IRC_USERNAME", "CBNGRelay_Dev"),
			Password: os.Getenv("CBNG_IRC_PASSWORD"),
			Channel: IrcRelayChannelConfiguration{
				Revert: envVarWithDefault("CBNG_IRC_CHANNEL_REVERT", "wikipedia-en-cbngrevertfeed2"),
				Debug:  envVarWithDefault("CBNG_IRC_CHANNEL_DEBUG", "wikipedia-en-cbngdebug2"),
			},
		},
		Sql: SqlInstanceConfiguration{
			Replica: []SqlConfiguration{
				SqlConfiguration{
					Username: os.Getenv("TOOL_REPLICA_USER"),
					Password: os.Getenv("TOOL_REPLICA_PASSWORD"),
					Host:     envVarWithDefault("TOOL_REPLICA_HOST", "enwiki.web.db.svc.wikimedia.cloud"),
					Port:     3306,
					Schema:   envVarWithDefault("TOOL_REPLICA_SCHEMA", "enwiki_p"),
				},
			},
			Cluebot: SqlConfiguration{
				Username: os.Getenv("TOOL_TOOLSDB_USER"),
				Password: os.Getenv("TOOL_TOOLSDB_PASSWORD"),
				Host:     envVarWithDefault("TOOL_TOOLSDB_HOST", "tools-db"),
				Port:     3306,
				Schema:   envVarWithDefault("TOOL_TOOLSDB_SCHEMA", "cluebotng"),
			},
		},
		Core: CoreConfiguration{
			Host: "core",
			Port: 3565,
		},
		Honey: HoneyConfiguration{
			Key:        envVarWithDefault("CBNG_HONEY_KEY", ""),
			SampleRate: 0.01,
		},
	}

	var configPath string
	if val, ok := os.LookupEnv("BOTNG_CFG"); ok {
		configPath = val
	}
	if configPath != "" {
		logger.Infof("Using configuration file %s", configPath)
		viper.SetConfigFile(configPath)

		if err := viper.ReadInConfig(); err != nil {
			logger.Fatalf("Error reading config file, %s", err)
		}
	}

	err := viper.Unmarshal(&configuration)
	if err != nil {
		logger.Fatalf("unable to decode into struct, %v", err)
	}
	return &configuration
}

func (c *Configuration) LoadDynamic(wg *sync.WaitGroup, wikipediaApi *wikipedia.WikipediaApi) {
	c.Instances.AngryOptInConfiguration = NewAngryOptInConfiguration(c, wikipediaApi, wg)
	c.Instances.TFA = NewTFA(c, wikipediaApi, wg)
	c.Instances.HuggleConfiguration = NewHuggleConfiguration(c, wikipediaApi, wg)
	c.Instances.NamespaceOptIn = NewNamespaceOptIn(c, wikipediaApi, wg)
	c.Instances.Run = NewRun(c, wikipediaApi, wg)
}
