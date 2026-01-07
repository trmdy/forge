package cli

import (
	"fmt"
	"math/rand"
	"strings"
	"time"
)

var (
	loopAdjectives = []string{
		"Agile", "Amped", "Atomic", "Beefy", "Bold", "Brisk", "Bright", "Brutal",
		"Crisp", "Daring", "Electric", "Epic", "Ferocious", "Fierce", "Flashy", "Focused",
		"Glorious", "Hyper", "Laser", "Legend", "Mighty", "Nimble", "Prime", "Rapid",
		"Rugged", "Savage", "Sharp", "Slick", "Solid", "Stellar", "Turbo", "Ultra",
	}
	loopNames = []string{
		"Homer", "Marge", "Bart", "Lisa", "Maggie", "Flanders", "Burns", "Smithers",
		"Milhouse", "Krusty", "Apu", "Lenny", "Carl", "Moe", "Wiggum", "Nelson",
	}
)

func generateLoopName(existing map[string]struct{}) string {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	maxAttempts := len(loopAdjectives) * len(loopNames) * 2

	for i := 0; i < maxAttempts; i++ {
		adjective := loopAdjectives[rng.Intn(len(loopAdjectives))]
		name := loopNames[rng.Intn(len(loopNames))]
		candidate := strings.TrimSpace(adjective + " " + name)
		if candidate == "" {
			continue
		}
		if _, ok := existing[candidate]; ok {
			continue
		}
		existing[candidate] = struct{}{}
		return candidate
	}

	fallback := "Loop " + time.Now().Format("150405")
	if _, ok := existing[fallback]; !ok {
		existing[fallback] = struct{}{}
		return fallback
	}

	counter := 1
	for {
		candidate := fmt.Sprintf("%s-%d", fallback, counter)
		if _, ok := existing[candidate]; !ok {
			existing[candidate] = struct{}{}
			return candidate
		}
		counter++
	}
}
