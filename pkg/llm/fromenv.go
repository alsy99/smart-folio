package llm

import (
	"log/slog"
	"os"
	"time"

	"aperture/pkg/config"
	"aperture/pkg/gemini"
	"aperture/pkg/httpx"
	"aperture/pkg/openai"
)

func FromEnv() Completer {
	httpClient := httpx.Client(20 * time.Second)
	var providers []named
	add := func(name string, c Completer) {
		if c != nil && c.Enabled() {
			slog.Info("llm ready", "provider", name)
			providers = append(providers, named{name: name, Completer: c})
		}
	}

	add("openai", wrapOpenAI(openai.FromEnv()))
	add("gemini", wrapGemini(gemini.FromEnv()))
	add("groq", &Compat{
		Name:    "groq",
		BaseURL: "https://api.groq.com/openai/v1/chat/completions",
		APIKey:  os.Getenv("GROQ_API_KEY"),
		Model:   config.String("GROQ_MODEL", "openai/gpt-oss-20b"),
		HTTP:    httpClient,
	})
	add("mistral", &Compat{
		Name:    "mistral",
		BaseURL: "https://api.mistral.ai/v1/chat/completions",
		APIKey:  os.Getenv("MISTRAL_API_KEY"),
		Model:   config.String("MISTRAL_MODEL", "mistral-small-latest"),
		HTTP:    httpClient,
	})
	add("openrouter", &Compat{
		Name:    "openrouter",
		BaseURL: "https://openrouter.ai/api/v1/chat/completions",
		APIKey:  os.Getenv("OPENROUTER_API_KEY"),
		Model:   config.String("OPENROUTER_MODEL", "meta-llama/llama-3.1-8b-instruct"),
		Headers: map[string]string{
			"HTTP-Referer": "http://127.0.0.1:43127",
			"X-Title":      "Aperture",
		},
		HTTP: httpClient,
	})
	add("nvidia", &Compat{
		Name:    "nvidia",
		BaseURL: "https://integrate.api.nvidia.com/v1/chat/completions",
		APIKey:  os.Getenv("NVIDIA_API_KEY"),
		Model:   config.String("NVIDIA_MODEL", "meta/llama-3.1-8b-instruct"),
		HTTP:    httpClient,
	})
	add("zai", &Compat{
		Name:    "zai",
		BaseURL: "https://api.z.ai/api/paas/v4/chat/completions",
		APIKey:  os.Getenv("ZAI_API_KEY"),
		Model:   config.String("ZAI_MODEL", "glm-4.5-flash"),
		HTTP:    httpClient,
	})

	if len(providers) == 0 {
		slog.Info("llm", "provider", "none")
		return openai.FromEnv()
	}
	return &Chain{providers: providers, Dir: Dir()}
}
