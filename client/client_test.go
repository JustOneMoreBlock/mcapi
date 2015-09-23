package mcapi

import (
	"testing"
)

func TestGetServerStatusGood(t *testing.T) {
	_, err := GetServerStatus("mc.syfaro.net", 25565)
	if err != nil {
		t.Log(err)
		t.Fail()
	}
}

func TestGetServerStatusBad(t *testing.T) {
	_, err := GetServerStatus("", -1)
	if err == nil {
		t.Log("Bad request was made and no error was created")
		t.Fail()
	}
}

func TestGetServerQueryGood(t *testing.T) {
	_, err := GetServerQuery("mc.syfaro.net", 25565)
	if err != nil {
		t.Log(err)
		t.Fail()
	}
}

func TestGetServerQueryBad(t *testing.T) {
	_, err := GetServerQuery("", -1)
	if err == nil {
		t.Log("Bad request was made and no error was created")
		t.Fail()
	}
}
