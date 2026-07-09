package vars

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"

	"github.com/kercre123/wire-pod/chipper/pkg/logger"
)

// a way to create a JSON configuration for wire-pod, rather than the use of env vars

var ApiConfigPath = "./apiConfig.json"

var APIConfig apiConfig

type apiConfig struct {
	Weather struct {
		Enable   bool   `json:"enable"`
		Provider string `json:"provider"`
		Key      string `json:"key"`
		Unit     string `json:"unit"`
	} `json:"weather"`
	Knowledge struct {
		Enable                 bool    `json:"enable"`
		Provider               string  `json:"provider"`
		Key                    string  `json:"key"`
		ID                     string  `json:"id"`
		Model                  string  `json:"model"`
		IntentGraph            bool    `json:"intentgraph"`
		RobotName              string  `json:"robotName"`
		OpenAIPrompt           string  `json:"openai_prompt"`
		OpenAIVoice            string  `json:"openai_voice"`
		OpenAIVoiceWithEnglish bool    `json:"openai_voice_with_english"`
		SaveChat               bool    `json:"save_chat"`
		CommandsEnable         bool    `json:"commands_enable"`
		Endpoint               string  `json:"endpoint"`
		TopP                   float32 `json:"top_p"`
		Temperature            float32 `json:"temp"`
	} `json:"knowledge"`
	STT struct {
		Service  string `json:"provider"`
		Language string `json:"language"`
	} `json:"STT"`
	TTS struct {
		Provider          string  `json:"provider"`
		TencentRegion     string  `json:"tencent_region"`
		TencentVoiceType  int64   `json:"tencent_voice_type"`
		TencentSampleRate int64   `json:"tencent_sample_rate"`
		TencentCodec      string  `json:"tencent_codec"`
		TencentSpeed      float64 `json:"tencent_speed"`
		TencentVolume     float64 `json:"tencent_volume"`
		TencentTimeoutSec int64   `json:"tencent_timeout_seconds"`
	} `json:"tts"`
	Server struct {
		// false for ip, true for escape pod
		EPConfig bool   `json:"epconfig"`
		Port     string `json:"port"`
	} `json:"server"`
	HasReadFromEnv   bool `json:"hasreadfromenv"`
	PastInitialSetup bool `json:"pastinitialsetup"`
}

func WriteConfigToDisk() {
	logger.Println("Configuration changed, writing to disk")
	writeBytes, _ := json.Marshal(APIConfig)
	os.WriteFile(ApiConfigPath, writeBytes, 0644)
}

func CreateConfigFromEnv() {
	// if no config exists, create it
	if os.Getenv("WEATHERAPI_ENABLED") == "true" {
		APIConfig.Weather.Enable = true
		APIConfig.Weather.Provider = os.Getenv("WEATHERAPI_PROVIDER")
		APIConfig.Weather.Key = os.Getenv("WEATHERAPI_KEY")
		APIConfig.Weather.Unit = os.Getenv("WEATHERAPI_UNIT")
	} else {
		APIConfig.Weather.Enable = false
	}
	if os.Getenv("KNOWLEDGE_ENABLED") == "true" {
		APIConfig.Knowledge.Enable = true
		APIConfig.Knowledge.Provider = os.Getenv("KNOWLEDGE_PROVIDER")
		if os.Getenv("KNOWLEDGE_PROVIDER") == "houndify" {
			APIConfig.Knowledge.ID = os.Getenv("KNOWLEDGE_ID")
		}
		APIConfig.Knowledge.Key = os.Getenv("KNOWLEDGE_KEY")
	} else {
		APIConfig.Knowledge.Enable = false
	}
	WriteSTT()
	ApplyTTSDefaults()
	APIConfig.HasReadFromEnv = true
	writeBytes, _ := json.Marshal(APIConfig)
	os.WriteFile(ApiConfigPath, writeBytes, 0644)
}

func WriteSTT() {
	// was not part of the original code, so this is its own function
	// launched if stt not found in config
	APIConfig.STT.Service = os.Getenv("STT_SERVICE")
	if os.Getenv("STT_SERVICE") == "vosk" || os.Getenv("STT_SERVICE") == "whisper.cpp" {
		APIConfig.STT.Language = os.Getenv("STT_LANGUAGE")
	}
}

func ReadConfig() {
	if _, err := os.Stat(ApiConfigPath); err != nil {
		CreateConfigFromEnv()
		logger.Println("API config JSON created")
	} else {
		// read config
		configBytes, err := os.ReadFile(ApiConfigPath)
		if err != nil {
			APIConfig.Knowledge.Enable = false
			APIConfig.Weather.Enable = false
			logger.Println("Failed to read API config file")
			logger.Println(err)
			return
		}
		err = json.Unmarshal(configBytes, &APIConfig)
		if err != nil {
			APIConfig.Knowledge.Enable = false
			APIConfig.Weather.Enable = false
			logger.Println("Failed to unmarshal API config JSON")
			logger.Println(err)
			return
		}
		// stt service is the only thing controlled by shell
		if APIConfig.STT.Service != os.Getenv("STT_SERVICE") {
			WriteSTT()
		}
		if !APIConfig.HasReadFromEnv {
			if APIConfig.Server.Port != os.Getenv("DDL_RPC_PORT") {
				APIConfig.HasReadFromEnv = true
				APIConfig.PastInitialSetup = true
			}
		}

		if APIConfig.Knowledge.Model == "meta-llama/Llama-2-70b-chat-hf" {
			logger.Println("Setting Together model to Llama3")
			APIConfig.Knowledge.Model = "meta-llama/Llama-3-70b-chat-hf"
		}
		ApplyTTSDefaults()

		writeBytes, _ := json.Marshal(APIConfig)
		os.WriteFile(ApiConfigPath, writeBytes, 0644)
		logger.Println("API config successfully read")
	}
}

func ApplyTTSDefaults() {
	if strings.TrimSpace(APIConfig.TTS.Provider) == "" {
		APIConfig.TTS.Provider = envOrDefault("TTS_PROVIDER", "tencent")
	}
	if strings.TrimSpace(APIConfig.TTS.TencentRegion) == "" {
		APIConfig.TTS.TencentRegion = envOrDefault("TENCENT_TTS_REGION", "ap-guangzhou")
	}
	if APIConfig.TTS.TencentVoiceType == 0 {
		APIConfig.TTS.TencentVoiceType = envInt64OrDefault("TENCENT_TTS_VOICE_TYPE", 601009)
	}
	if APIConfig.TTS.TencentSampleRate == 0 {
		APIConfig.TTS.TencentSampleRate = envInt64OrDefault("TENCENT_TTS_SAMPLE_RATE", 16000)
	}
	if strings.TrimSpace(APIConfig.TTS.TencentCodec) == "" {
		APIConfig.TTS.TencentCodec = strings.ToLower(envOrDefault("TENCENT_TTS_CODEC", "pcm"))
	}
	if APIConfig.TTS.TencentTimeoutSec == 0 {
		APIConfig.TTS.TencentTimeoutSec = envInt64OrDefault("TENCENT_TTS_TIMEOUT_SECONDS", 15)
	}
	if envValue := strings.TrimSpace(os.Getenv("TENCENT_TTS_SPEED")); envValue != "" {
		APIConfig.TTS.TencentSpeed = envFloat64OrDefault("TENCENT_TTS_SPEED", APIConfig.TTS.TencentSpeed)
	}
	if envValue := strings.TrimSpace(os.Getenv("TENCENT_TTS_VOLUME")); envValue != "" {
		APIConfig.TTS.TencentVolume = envFloat64OrDefault("TENCENT_TTS_VOLUME", APIConfig.TTS.TencentVolume)
	}
}

func envOrDefault(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func envInt64OrDefault(key string, fallback int64) int64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		logger.Println("Invalid " + key + ", using default " + strconv.FormatInt(fallback, 10))
		return fallback
	}
	return parsed
}

func envFloat64OrDefault(key string, fallback float64) float64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		logger.Println("Invalid " + key + ", using default")
		return fallback
	}
	return parsed
}
