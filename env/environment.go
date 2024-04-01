package env

import (
	"sync"

	"go.mongodb.org/mongo-driver/mongo"
)

var envInstance *Environment
var createEnvironmentOnce sync.Once

type Environment struct {
	ServicePort int
}

func Initialize() {
	createEnvironmentOnce.Do(func() {
		envInstance = createEnvironment()
	})
}

func Instance() *Environment {
	Initialize()
	return envInstance
}

func createEnvironment() *Environment {
	setUpZap()
	return &Environment{
		ServicePort: loadServicePort(),
	}
}

type Mgo struct {
	Host        []string
	AdminDbName string
	MinPoolSize int
	PoolLimit   int
	UserName    string
	Password    string
	DbName      string
	Client      *mongo.Client
}

type MongoDbCollections struct {
	StockBasic string
}
