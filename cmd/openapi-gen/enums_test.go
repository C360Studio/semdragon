package main

import (
	"reflect"
	"testing"
)

func TestBossBattleStatusEnumExcludesDeletedRetreatStatus(t *testing.T) {
	got := enumFieldOverrides["BossBattle"]["status"]
	want := []any{"active", "victory", "defeat"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BossBattle.status enum = %v, want %v", got, want)
	}
}
