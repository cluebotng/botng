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
}

func NewConfiguration() *Configuration {
	logger := logrus.WithField("function", "config.NewConfiguration")
	configuration := Configuration{}

	var configPath string
	if val, ok := os.LookupEnv("BOTNG_CFG"); ok {
		configPath = val
	}
	if configPath != "" {
		viper.SetConfigFile(configPath)
	} else {
		viper.SetConfigName("config")
		viper.AddConfigPath(".")
	}
	if err := viper.ReadInConfig(); err != nil {
		logger.Fatalf("Error reading config file, %s", err)
	}

	configuration.Runtime.Release = ReleaseTag
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
