package runner

import "github.com/dotbrains/bellows/internal/config"

func Resolve(label string, cfg config.RunnerConfig) (string, bool) {
	if image, ok := cfg.Map[label]; ok && image != "" {
		return image, true
	}
	if image, ok := config.DefaultRunnerImage(label); ok {
		return image, true
	}
	if cfg.DefaultImage != "" {
		return cfg.DefaultImage, true
	}
	return "", false
}

func IsBlacksmith(label string) bool {
	return len(label) >= len("blacksmith-") && label[:len("blacksmith-")] == "blacksmith-"
}
