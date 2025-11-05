package relay

import (
	"bufio"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"github.com/cluebotng/botng/pkg/cbng/config"
	"github.com/cluebotng/botng/pkg/cbng/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"golang.org/x/time/rate"
	"net"
	"strings"
	"sync"
)

type Relays struct {
	revert *IrcServer
	spam   *IrcServer
	debug  *IrcServer
}

type IrcServer struct {
	host            string
	nick            string
	username        string
	password        string
	port            int
	channel         string
	connection      net.Conn
	scanner         *bufio.Scanner
	sendChan        chan string
	reConnectSignal chan bool
	limiter         *rate.Limiter
}

func (f *IrcServer) connect() {
	logger := logrus.WithFields(logrus.Fields{
		"function": "relay.IrcServer.connect",
		"args": map[string]interface{}{
			"nick": f.nick,
		},
	})

	if f.connection == nil {
		logger.Tracef("Connecting to IRC server")
		conn, err := tls.Dial("tcp", fmt.Sprintf("%s:%d", f.host, f.port), &tls.Config{})
		if err != nil {
			logger.Errorf("IRC error: %v\n", err)
			f.close()
			return
		}
		f.connection = conn
		f.scanner = bufio.NewScanner(f.connection)
	}
}

func (f *IrcServer) close() {
	logger := logrus.WithFields(logrus.Fields{
		"function": "relay.IrcServer.close",
		"args": map[string]interface{}{
			"nick": f.nick,
		},
	})

	logger.Info("Closing connection to IRC server")
	if f.connection != nil {
		if err := f.connection.Close(); err != nil {
			logger.Errorf("Error closing IRC connection: %v", err)
		}
	}
	f.connection = nil
}

func (f *IrcServer) send(message string) bool {
	upperCaseMessage := strings.ToUpper(message)
	isPrivate := strings.HasPrefix(upperCaseMessage, "PRIVMSG NICKSERV ") || strings.HasPrefix(upperCaseMessage, "AUTHENTICATE ")
	loggerMessage := message
	if isPrivate {
		loggerMessage = "**secret**"
	}

	logger := logrus.WithFields(logrus.Fields{
		"function": "relay.IrcServer.send",
		"args": map[string]interface{}{
			"nick":    f.nick,
			"message": loggerMessage,
		},
	})

	if f.connection == nil {
		logger.Warnf("Failed to write due to no connection: %+v", message)
		return false
	}

	// Don't log passwords
	if isPrivate {
		logger.Debugf("Sending to IRC: **secret**")
	} else {
		logger.Debugf("Sending to IRC: %+v", message)
	}
	if _, err := fmt.Fprintf(f.connection, "%s\n", message); err != nil {
		return false
	}
	return true
}

func NewRelays(wg *sync.WaitGroup, enableIrc bool, host string, port int, nick, password string, channels config.IrcRelayChannelConfiguration) *Relays {
	servers := map[string]*IrcServer{}
	if enableIrc {
		for _, relayType := range []string{"debug", "revert"} {
			var channel string
			switch relayType {
			case "debug":
				channel = channels.Debug
			case "revert":
				channel = channels.Revert
			default:
				logrus.Panicf("Unknown relay type: %+v", relayType)

			}

			f := IrcServer{
				host:            host,
				port:            port,
				nick:            fmt.Sprintf("%v-%v", nick, relayType),
				username:        nick,
				password:        password,
				sendChan:        make(chan string, 10000),
				reConnectSignal: make(chan bool, 1),
				limiter:         rate.NewLimiter(2, 4),
				channel:         channel,
			}
			wg.Add(1)
			go f.reader(wg)
			f.connect()
			wg.Add(1)
			go f.reconnector(wg)
			servers[relayType] = &f
		}
	}

	r := Relays{
		spam:   servers["spam"],
		revert: servers["revert"],
		debug:  servers["debug"],
	}
	return &r
}

func (r *Relays) SendDebug(message string) {
	metrics.IrcNotificationsSent.With(prometheus.Labels{"channel": "debug"}).Inc()
	if r.debug != nil {
		r.debug.sendChan <- message
	}
}

func (r *Relays) SendRevert(message string) {
	metrics.IrcNotificationsSent.With(prometheus.Labels{"channel": "revert"}).Inc()
	if r.revert != nil {
		r.revert.sendChan <- message
	}
}

func (f *IrcServer) reader(wg *sync.WaitGroup) {
	defer wg.Done()
	logger := logrus.WithFields(logrus.Fields{
		"function": "relay.IrcServer.reader",
		"args": map[string]interface{}{
			"nick": f.nick,
		},
	})

	nickCount := 0
	currentNick := strings.ReplaceAll(f.nick, " ", "-")
	hasDoneTheSaslDance := false
	for {
		if f.connection == nil {
			f.reConnectSignal <- true
			if f.connection == nil {
				continue
			}
		}

		f.scanner.Scan()
		line := f.scanner.Text()
		lparts := strings.Split(line, " ")
		llogger := logger.WithFields(logrus.Fields{
			"line":       line,
			"line_parts": lparts,
		})
		llogger.Trace("Parsing line")

		switch {
		// Authenticate early
		case strings.HasSuffix(line, "*** No Ident response"):
			if f.password != "" {
				// Prefer to do SASL
				f.send("CAP REQ :sasl")
			}
			f.send(fmt.Sprintf("USER %s \"1\" \"1\" :ClueBot Wikipedia Bot 3.0.", strings.ReplaceAll(f.nick, " ", "_")))
			f.send(fmt.Sprintf("NICK %s", strings.ReplaceAll(f.nick, " ", "_")))

		case strings.HasSuffix(line, "CAP * ACK :sasl"):
			f.send("AUTHENTICATE PLAIN")

		case strings.HasSuffix(line, "AUTHENTICATE +"):
			authPayload := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s\x00%s\x00%s", f.username, f.username, f.password)))
			f.send(fmt.Sprintf("AUTHENTICATE %s", authPayload))

		case strings.HasSuffix(line, "SASL authentication successful"):
			f.send("CAP END")
			hasDoneTheSaslDance = true

		case lparts[0] == "ERROR":
			llogger.Errorf("Received error: %v", line)
			f.close()
		case lparts[0] == "PING":
			f.send(fmt.Sprintf("PONG %s", strings.TrimLeft(lparts[1], ":")))
		case len(lparts) >= 2 && (lparts[1] == "376" || lparts[1] == "422"):
			if f.password != "" && !hasDoneTheSaslDance {
				f.send(fmt.Sprintf("PRIVMSG NickServ :IDENTIFY %s %s", currentNick, f.password))
			}
			f.send(fmt.Sprintf("JOIN #%s", f.channel))
			go f.writer(wg)
		case len(lparts) >= 2 && lparts[1] == "433":
			nickCount++
			currentNick = fmt.Sprintf("%s_%d", currentNick, nickCount)
			llogger.Warnf("Nick already in use - trying %s", currentNick)
			f.send(fmt.Sprintf("NICK %s\n", currentNick))
		default:
			llogger.Tracef("Unsupported IRC event: %+v", lparts)
		}
	}
}

func (f *IrcServer) writer(wg *sync.WaitGroup) {
	defer wg.Done()
	logger := logrus.WithFields(logrus.Fields{
		"function": "relay.IrcServer.writer",
		"args": map[string]interface{}{
			"nick": f.nick,
		},
	})

	logger.Tracef("Started IRC writer")
	for {
		// We will spawn a new writer on every connection
		if f.connection == nil {
			logger.Warn("Stopping writing due to no connection")
			break
		}
		message := <-f.sendChan
		// Just drop the message if we cannot send right now,
		// otherwise we just make a huge backlog
		if f.limiter.Allow() {
			logger.Tracef("Sending: %+v\n", message)
			if !f.send(fmt.Sprintf("PRIVMSG #%s :%s", f.channel, message)) {
				logger.Warn("IRC write error")
				break
			}
		} else {
			logger.Tracef("Not sending due to rate limit: %+v\n", message)
		}
	}
	f.close()
}

func (f *IrcServer) reconnector(wg *sync.WaitGroup) {

	defer wg.Done()
	for {
		<-f.reConnectSignal
		f.connect()
	}
}

func (r *Relays) GetPendingDebugMessages() int {
	if r.debug == nil {
		return 0
	}
	return len(r.debug.sendChan)
}

func (r *Relays) GetPendingRevertMessages() int {
	if r.revert == nil {
		return 0
	}
	return len(r.revert.sendChan)
}
