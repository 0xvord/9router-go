package providers

import "sort"

// ProviderAliasMap maps short aliases to canonical provider IDs.
var ProviderAliasMap = map[string]string{
	"aai":            "assemblyai",
	"ag":             "antigravity",
	"ali":            "alicode",
	"ali-tp":         "alitp-intl",
	"alii":           "alicode-intl",
	"alitp":          "alitp-intl",
	"ant":            "anthropic",
	"ark":            "volcengine-ark",
	"az":             "azure",
	"bb":             "blackbox",
	"bfl":            "black-forest-labs",
	"bpm":            "byteplus",
	"brave":          "brave-search",
	"cb":             "cerebras",
	"cbai":           "codebuddy-intl",
	"cc":             "claude",
	"cd":             "codebuddy-cn",
	"cbcn":           "codebuddy-cn",
	"cf":             "cloudflare-ai",
	"ch":             "chutes",
	"cl":             "cline",
	"cmc":            "commandcode",
	"cp":             "clinepass",
	"cu":             "cursor",
	"cx":             "codex",
	"dg":             "deepgram",
	"ds":             "deepseek",
	"el":             "elevenlabs",
	"fal":            "fal-ai",
	"fb":             "freebuff",
	"fish":           "fish-audio",
	"fl":             "featherless",
	"fw":             "fireworks",
	"gb":             "grok-cli",
	"gc":             "gemini-cli",
	"gcli":           "grok-cli",
	"gh":             "github",
	"gl":             "gitlab",
	"glmcn":          "glm-cn",
	"gpse":           "google-pse",
	"gq":             "groq",
	"grok-build":     "grok-cli",
	"gw":             "grok-web",
	"hf":             "huggingface",
	"hyp":            "hyperbolic",
	"if":             "iflow",
	"jina":           "jina-ai",
	"kc":             "kilocode",
	"km":             "kimi",
	"kr":             "kiro",
	"mimo":           "xiaomi-mimo",
	"mm":             "minimax",
	"mmf":            "mimo-free",
	"nb":             "nanobanana",
	"ne":             "nebius",
	"nv":             "nvidia",
	"oa":             "openai",
	"oc":             "opencode",
	"ocz":            "opencode-zen",
	"or":             "openrouter",
	"pa":             "perplexity-agent",
	"polly":          "aws-polly",
	"pplx":           "perplexity",
	"pplx-agent":     "perplexity-agent",
	"pplx-responses": "perplexity-agent",
	"pw":             "perplexity-web",
	"qd":             "qoder",
	"runway":         "runwayml",
	"stability":      "stability-ai",
	"tg":             "together",
	"vali":           "volcengine-ark",
	"vercel":         "vercel-ai-gateway",
	"vn":             "venice",
	"xmtp":           "xiaomi-tokenplan",
	"af":             "api-airforce",
	"bzl":            "bazaarlink",
	"bm":             "bluesminds",
	"dv":             "devin-cli",
	"hunyuan":        "tencent",
	"kgw":            "kilo-gateway",
	"ps":             "poolside",
	"qianfan":        "baidu",
	"samba":          "sambanova",
	"tr":             "trae",
	"voyage":         "voyage-ai",
	"vx":             "vertex",
	"vxp":            "vertex-partner",
	"ws":             "windsurf",
	"xq":             "xquik",
	"zd":             "zed",
}

// ResolveAlias returns the canonical provider ID for an alias, or the alias itself if not found.
func ResolveAlias(alias string) string {
	if canonical, ok := ProviderAliasMap[alias]; ok {
		return canonical
	}
	return alias
}

// ProviderToAliasMap maps canonical provider IDs to their primary short alias.
var ProviderToAliasMap = map[string]string{}

// CatalogEmitAlias mirrors Next.js wG(): canonical provider -> the EXACT alias
// its /v1/models entries must carry. Ground truth (live 0.5.86 /v1/models):
//   codebuddy-cn -> cbcn, grok-cli -> gcli, antigravity -> ag, commandcode -> cmc
// Emission and reverse-alias pick these first; sorted fallback covers the rest.
var CatalogEmitAlias = map[string]string{
	"codebuddy-cn": "cbcn",
	"grok-cli":     "gcli",
	"antigravity":  "ag",
	"commandcode":  "cmc",
}

func init() {
	// Deterministic: catalog aliases first, then sorted alias order (first-wins).
	// Go map iteration is randomized — never build the reverse map directly from it.
	for provider, alias := range CatalogEmitAlias {
		if _, exists := ProviderToAliasMap[provider]; !exists {
			ProviderToAliasMap[provider] = alias
		}
	}
	keys := make([]string, 0, len(ProviderAliasMap))
	for a := range ProviderAliasMap {
		keys = append(keys, a)
	}
	sort.Strings(keys)
	for _, alias := range keys {
		provider := ProviderAliasMap[alias]
		if _, exists := ProviderToAliasMap[provider]; !exists {
			ProviderToAliasMap[provider] = alias
		}
	}
}

// GetProviderAlias returns the primary short alias for a canonical provider ID, or providerID itself.
func GetProviderAlias(providerID string) string {
	if alias, ok := ProviderToAliasMap[providerID]; ok && alias != "" {
		return alias
	}
	return providerID
}
