package cmd

import (
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"log"
	"testing"
)

func initTestConfig(t *testing.T) {
	viper.SetConfigFile("testdata/config.yaml")
	err := viper.ReadInConfig()
	require.Nil(t, err)
}

func TestRoot(t *testing.T) {
	initTestConfig(t)
}

func TestLoadLibrary(t *testing.T) {
	dll := "api-ms-win-crt-environment-l1-1-0.dll"
	path, err := GetDLLPath(dll)
	require.Nil(t, err)
	log.Printf("path=%s\n", path)
}
