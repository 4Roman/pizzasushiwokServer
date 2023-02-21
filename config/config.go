package config

import (
	"fmt"
	"github.com/kelseyhightower/envconfig"
	"sync"
)

type Configuration struct {
	DbHost     string `envconfig:"DB_HOST" default:"localhost"`
	DbPort     string `envconfig:"DB_PORT" default:"5432"`
	DbName     string `envconfig:"DB_NAME" default:"asteroids"`
	DbTable    string `envconfig:"DB_NAME" default:"neo_count"`
	DbUser     string `envconfig:"DB_USER" default:"Roman"`
	DbPassword string `envconfig:"DB_PASSWORD" default:"Roman2002"`
	NasaApiKey string `envconfig:"NASA_API_KEY" default:"Xd6ej4mitI5sa4HOpXJ0f26xy6kqNvvcCHatPihk"`
}

var lock = &sync.Mutex{}
var singleInstance *Configuration

func GetInstance() *Configuration {
	if singleInstance == nil {
		lock.Lock()
		defer lock.Unlock()
		if singleInstance == nil {
			singleInstance = new(Configuration)
			if err := envconfig.Process("", singleInstance); err != nil {
				fmt.Println("failed to load envconfig")
			}
			return singleInstance
		}
	}
	return singleInstance
}
func GetApiKey() string {
	if singleInstance != nil {
		return singleInstance.NasaApiKey
	} else {
		return GetInstance().NasaApiKey
	}

}
