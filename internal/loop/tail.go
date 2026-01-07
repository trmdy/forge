package loop

import (
	"bufio"
	"os"
	"strings"
)

func tailFile(path string, maxLines int) (string, error) {
	if maxLines <= 0 {
		return "", nil
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lines := make([]string, 0, maxLines)
	for scanner.Scan() {
		if len(lines) >= maxLines {
			lines = lines[1:]
		}
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}

	return strings.Join(lines, "\n"), nil
}
