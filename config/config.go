package config

import (
	gotenv "github.com/subosito/gotenv"
)

func LoadEnvs() {
	gotenv.Load(".env")
}
