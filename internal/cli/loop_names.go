package cli

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/tOgg1/forge/internal/names"
)

func generateLoopName(existing map[string]struct{}) string {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	maxAttempts := names.LoopNameCountTwoPart() * 2

	for i := 0; i < maxAttempts; i++ {
		candidate := strings.TrimSpace(names.RandomLoopNameTwoPart(rng))
		if candidate == "" {
			continue
		}
		if _, ok := existing[candidate]; ok {
			continue
		}
		return candidate
	}

	maxAttempts = names.LoopNameCountThreePart() * 2
	for i := 0; i < maxAttempts; i++ {
		candidate := strings.TrimSpace(names.RandomLoopNameThreePart(rng))
		if candidate == "" {
			continue
		}
		if _, ok := existing[candidate]; ok {
			continue
		}
		return candidate
	}

	fallback := "loop-" + time.Now().Format("150405")
	if _, ok := existing[fallback]; !ok {
		return fallback
	}

	counter := 1
	for {
		candidate := fmt.Sprintf("%s-%d", fallback, counter)
		if _, ok := existing[candidate]; !ok {
			return candidate
		}
		counter++
	}
}
