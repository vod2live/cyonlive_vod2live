package playout

import (
	"encoding/json"
	"github.com/google/uuid"
	"testing"
)

func TestStateCarriesFencingToken(t *testing.T) {
	state := State{ChannelID: uuid.New(), Status: "running", FencingToken: 42}
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	var decoded State
	if err = json.Unmarshal(raw, &decoded); err != nil || decoded.FencingToken != 42 {
		t.Fatalf("decoded=%+v err=%v", decoded, err)
	}
}
