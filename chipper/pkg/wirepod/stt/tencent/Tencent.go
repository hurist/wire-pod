package wirepod_tencent

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/kercre123/wire-pod/chipper/pkg/logger"
	sr "github.com/kercre123/wire-pod/chipper/pkg/wirepod/speechrequest"
	asr "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/asr/v20190614"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	tcerrors "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/errors"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
)

var Name string = "tencent"

var (
	tencentClient *asr.Client
	tencentConfig tencentASRConfig
)

type tencentASRConfig struct {
	SecretID        string
	SecretKey       string
	Region          string
	EngineModelType string
	VoiceFormat     string
	FilterDirty     *int64
	FilterModal     *int64
	FilterPunc      *int64
	ConvertNumMode  *int64
	Timeout         time.Duration
}

func Init() error {
	cfg, err := loadTencentASRConfig()
	if err != nil {
		logger.Println("Tencent ASR init failed: " + err.Error())
		return err
	}

	credential := common.NewCredential(cfg.SecretID, cfg.SecretKey)
	clientProfile := profile.NewClientProfile()
	clientProfile.HttpProfile.Endpoint = "asr.tencentcloudapi.com"
	clientProfile.HttpProfile.ReqTimeout = int(cfg.Timeout.Seconds())

	client, err := asr.NewClient(credential, cfg.Region, clientProfile)
	if err != nil {
		logger.Println("Tencent ASR client init failed: " + err.Error())
		return err
	}

	tencentConfig = cfg
	tencentClient = client
	logger.Println("Tencent ASR client initialized")
	return nil
}

func loadTencentASRConfig() (tencentASRConfig, error) {
	cfg := tencentASRConfig{
		SecretID:        strings.TrimSpace(os.Getenv("TENCENTCLOUD_SECRET_ID")),
		SecretKey:       strings.TrimSpace(os.Getenv("TENCENTCLOUD_SECRET_KEY")),
		Region:          envOrDefault("TENCENT_ASR_REGION", "ap-guangzhou"),
		EngineModelType: envOrDefault("TENCENT_ASR_ENGINE_MODEL_TYPE", "16k_zh"),
		VoiceFormat:     strings.ToLower(envOrDefault("TENCENT_ASR_VOICE_FORMAT", "pcm")),
		Timeout:         time.Duration(envInt64OrDefault("TENCENT_ASR_TIMEOUT_SECONDS", 15)) * time.Second,
	}
	if cfg.SecretID == "" {
		return cfg, errors.New("TENCENTCLOUD_SECRET_ID is required for Tencent ASR")
	}
	if cfg.SecretKey == "" {
		return cfg, errors.New("TENCENTCLOUD_SECRET_KEY is required for Tencent ASR")
	}
	cfg.FilterDirty = optionalEnvInt64("TENCENT_ASR_FILTER_DIRTY")
	cfg.FilterModal = optionalEnvInt64("TENCENT_ASR_FILTER_MODAL")
	cfg.FilterPunc = optionalEnvInt64("TENCENT_ASR_FILTER_PUNC")
	cfg.ConvertNumMode = optionalEnvInt64("TENCENT_ASR_CONVERT_NUM_MODE")
	return cfg, nil
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
	if err != nil || parsed <= 0 {
		logger.Println("Invalid " + key + ", using default " + fmt.Sprint(fallback))
		return fallback
	}
	return parsed
}

func optionalEnvInt64(key string) *int64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		logger.Println("Invalid " + key + ", ignoring")
		return nil
	}
	return &parsed
}

func buildSentenceRecognitionRequest(cfg tencentASRConfig, audio []byte, audioKey string) (*asr.SentenceRecognitionRequest, error) {
	if len(audio) < 320 {
		return nil, fmt.Errorf("Tencent ASR audio is empty or too short: %d bytes", len(audio))
	}

	request := asr.NewSentenceRecognitionRequest()
	projectID := uint64(0)
	subServiceType := uint64(2)
	sourceType := uint64(1)
	dataLen := int64(len(audio))
	encodedAudio := base64.StdEncoding.EncodeToString(audio)

	request.ProjectId = &projectID
	request.SubServiceType = &subServiceType
	request.EngSerViceType = &cfg.EngineModelType
	request.SourceType = &sourceType
	request.VoiceFormat = &cfg.VoiceFormat
	request.UsrAudioKey = &audioKey
	request.Data = &encodedAudio
	request.DataLen = &dataLen
	request.FilterDirty = cfg.FilterDirty
	request.FilterModal = cfg.FilterModal
	request.FilterPunc = cfg.FilterPunc
	request.ConvertNumMode = cfg.ConvertNumMode

	return request, nil
}

func collectSpeechAudio(req *sr.SpeechRequest) error {
	for {
		_, err := req.GetNextStreamChunk()
		if err != nil {
			return err
		}
		speechIsDone, _ := req.DetectEndOfSpeech()
		if speechIsDone {
			return nil
		}
	}
}

func STT(req sr.SpeechRequest) (string, error) {
	logger.Println("(Bot " + req.Device + ", Tencent ASR) Processing...")
	if tencentClient == nil {
		return "", errors.New("Tencent ASR client is not initialized")
	}

	if err := collectSpeechAudio(&req); err != nil {
		return "", err
	}
	audio := req.DecodedMicData
	audioKey := req.Device + "-" + req.Session + "-" + fmt.Sprint(time.Now().UnixNano())
	request, err := buildSentenceRecognitionRequest(tencentConfig, audio, audioKey)
	if err != nil {
		logger.Println(err)
		return "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), tencentConfig.Timeout)
	defer cancel()

	response, err := tencentClient.SentenceRecognitionWithContext(ctx, request)
	if err != nil {
		logTencentError(err)
		return "", err
	}
	if response == nil || response.Response == nil || response.Response.Result == nil {
		return "", errors.New("Tencent ASR response did not include a result")
	}

	transcribedText := strings.ToLower(strings.TrimSpace(*response.Response.Result))
	logger.Println("Bot " + req.Device + " Transcribed text: " + transcribedText)
	return transcribedText, nil
}

func logTencentError(err error) {
	if sdkErr, ok := err.(*tcerrors.TencentCloudSDKError); ok {
		logger.Println("Tencent ASR error: RequestId=" + sdkErr.GetRequestId() + " Code=" + sdkErr.GetCode() + " Message=" + sdkErr.GetMessage())
		return
	}
	logger.Println("Tencent ASR error: " + err.Error())
}
