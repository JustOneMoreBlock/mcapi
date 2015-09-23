package mcapi

import (
	"fmt"
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

func ExampleGetServerStatus() {
	status, err := GetServerStatus("mc.syfaro.net", 25565)
	if err != nil {
		panic(err)
	}

	fmt.Printf("You have %d/%d players online!", status.Players.Now, status.Players.Max)

	// Output:
	// You have 0/20 players online!
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

func ExampleGetServerQuery() {
	status, err := GetServerQuery("mc.syfaro.net", 25565)
	if err != nil {
		panic(err)
	}

	fmt.Printf("You have %d/%d players on a server running %s!", status.Players.Now, status.Players.Max, status.ServerMod)

	// Output:
	// You have 0/20 players on a server running CraftBukkit on Bukkit 1.8.3-R0.1-SNAPSHOT!
}
