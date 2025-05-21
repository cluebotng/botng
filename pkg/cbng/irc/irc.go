package irc

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"github.com/sirupsen/logrus"
	"golang.org/x/time/rate"
	"net"
	"strings"
	"sync"
)

type IrcServer struct {
	Host            string
	Nick            string
	Password        string
	Port            int
	Channel         string
	UseTLS          bool
	connection      net.Conn
	scanner         *bufio.Scanner
	SendChan        chan string
	RecvChan        chan string
	ReConnectSignal chan bool
	Limiter         *rate.Limiter
}

func (f *IrcServer) Connect() {
	logger := logrus.WithFields(logrus.Fields{
		"function": "relay.IrcServer.connect",
		"args": map[string]interface{}{
			"nick": f.Nick,
		},
	})

	if f.connection == nil {
		logger.Info("Connecting to IRC server")
		if f.UseTLS {
			conn, err := tls.Dial("tcp", fmt.Sprintf("%s:%d", f.Host, f.Port), &tls.Config{})
			if err != nil {
				logger.Errorf("IRC error: %v\n", err)
				f.close()
				return
			}
			f.connection = conn
		} else {
			tcpAddr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:%d", f.Host, f.Port))
			if err != nil {
				logger.Errorf("TCP resolve error: %v\n", err)
				f.close()
				return
			}

			conn, err := net.DialTCP("tcp", nil, tcpAddr)
			if err != nil {
				logger.Errorf("IRC error: %v\n", err)
				f.close()
				return
			}
			f.connection = conn
		}
		f.scanner = bufio.NewScanner(f.connection)
		f.send(fmt.Sprintf("USER %s \"1\" \"1\" :ClueBot Wikipedia Bot 3.0.", strings.ReplaceAll(f.Nick, " ", "_")))
		f.send(fmt.Sprintf("NICK %s", strings.ReplaceAll(f.Nick, " ", "_")))
	}
}

func (f *IrcServer) close() {
	logger := logrus.WithFields(logrus.Fields{
		"function": "relay.IrcServer.close",
		"args": map[string]interface{}{
			"nick": f.Nick,
		},
	})

	logger.Info("Closing connection to IRC server")
	if f.connection != nil {
		f.connection.Close()
	}
	f.connection = nil
}

func (f *IrcServer) send(message string) bool {
	isPrivate := strings.Contains(strings.ToUpper(message), "PRIVMSG NICKSERV")
	loggerMessage := message
	if isPrivate {
		loggerMessage = "**secret**"
	}

	logger := logrus.WithFields(logrus.Fields{
		"function": "relay.IrcServer.send",
		"args": map[string]interface{}{
			"nick":    f.Nick,
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

func (f *IrcServer) Reader(wg *sync.WaitGroup) {
	logger := logrus.WithFields(logrus.Fields{
		"function": "relay.IrcServer.reader",
		"args": map[string]interface{}{
			"nick": f.Nick,
		},
	})

	wg.Add(1)
	defer wg.Done()

	nickCount := 0
	for {
		if f.connection == nil {
			f.ReConnectSignal <- true
			if f.connection == nil {
				continue
			}
		}

		f.scanner.Scan()
		line := f.scanner.Text()
		lparts := strings.Split(line, " ")
		line_logger := logger.WithFields(logrus.Fields{
			"line":       line,
			"line_parts": lparts,
		})
		line_logger.Trace("Parsing line")

		switch {
		case lparts[0] == "ERROR":
			line_logger.Errorf("Received error: %v", line)
			f.close()
		case lparts[0] == "PING":
			f.send(fmt.Sprintf("PING %s", strings.TrimLeft(lparts[1], ":")))
		case len(lparts) >= 2 && (lparts[1] == "376" || lparts[1] == "422"):
			if f.Password != "" {
				f.send(fmt.Sprintf("PRIVMSG NickServ :IDENTIFY %s %s", strings.ReplaceAll(f.Nick, " ", "_"), f.Password))
			}
			f.send(fmt.Sprintf("JOIN #%s", f.Channel))
			if f.SendChan != nil {
				go f.writer(wg)
			}
		case len(lparts) >= 2 && lparts[1] == "433":
			line_logger.Warnf("Nick already in use")
			nickCount += 1
			f.send(fmt.Sprintf("NICK %s_%d\n", strings.ReplaceAll(f.Nick, " ", "_"), nickCount))
		case len(lparts) >= 2 && lparts[1] == "PRIVMSG":
			if f.RecvChan != nil {
				select {
				case f.RecvChan <- strings.Join(lparts[2:], " "):
				default:
					logger.Errorf("Failed to write to receive channel")
				}
			}
		default:
			line_logger.Tracef("Unsupported IRC event: %+v", lparts)
		}
	}
}

func (f *IrcServer) writer(wg *sync.WaitGroup) {
	logger := logrus.WithFields(logrus.Fields{
		"function": "relay.IrcServer.writer",
		"args": map[string]interface{}{
			"nick": f.Nick,
		},
	})

	wg.Add(1)
	defer wg.Done()
	logger.Info("Started IRC writer")
	for {
		// We will spawn a new writer on every connection
		if f.connection == nil {
			logger.Warn("Stopping writing due to no connection")
			break
		}
		if f.Limiter.Allow() {
			message := <-f.SendChan
			logger.Infof("Sending: %+v\n", message)
			if !f.send(fmt.Sprintf("PRIVMSG #%s :%s", f.Channel, message)) {
				logger.Warn("IRC write error")
				break
			}
		} else {
			logger.Infof("Not permitted to write")
		}
	}
	f.close()
}

func (f *IrcServer) Reconnector(wg *sync.WaitGroup) {
	wg.Add(1)
	defer wg.Done()
	for range f.ReConnectSignal {
		f.Connect()
	}
}
