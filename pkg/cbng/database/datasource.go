package database

import (
	"github.com/cluebotng/botng/pkg/cbng/config"
	"github.com/cluebotng/botng/pkg/cbng/database/cluebot"
	"github.com/cluebotng/botng/pkg/cbng/database/replica"
)

type DatabaseConnection struct {
	Replica *replica.ReplicaInstance
	ClueBot *cluebot.CluebotInstance
}

func NewDatabaseConnection(configuration *config.Configuration) *DatabaseConnection {
	c := DatabaseConnection{
		Replica: replica.NewReplicaInstance(configuration),
		ClueBot: cluebot.NewCluebotInstance(configuration),
	}
	return &c
}
