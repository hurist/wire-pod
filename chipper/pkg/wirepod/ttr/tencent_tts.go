package wirepod_ttr

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/fforchino/vector-go-sdk/pkg/vector"
	"github.com/google/uuid"
	"github.com/kercre123/wire-pod/chipper/pkg/logger"
	"github.com/kercre123/wire-pod/chipper/pkg/vars"
	tts "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/tts/v20190823"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	tcerrors "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/errors"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
)

type tencentTTSConfig struct {
	SecretID   string
	SecretKey  string
	Region     string
	VoiceType  int64
	SampleRate int64
	Codec      string
	Speed      float64
	Volume     float64
	Timeout    time.Duration
}

func DoSayText_Tencent(robot *vector.Vector, input string) error {
	if strings.TrimSpace(input) == "" {
		return nil
	}
	cfg, err := loadTencentTTSConfig()
	if err != nil {
		return err
	}

	credential := common.NewCredential(cfg.SecretID, cfg.SecretKey)
	clientProfile := profile.NewClientProfile()
	clientProfile.HttpProfile.Endpoint = "tts.tencentcloudapi.com"
	clientProfile.HttpProfile.ReqTimeout = int(cfg.Timeout.Seconds())
	client, err := tts.NewClient(credential, cfg.Region, clientProfile)
	if err != nil {
		return err
	}

	request, err := buildTencentTTSRequest(cfg, input)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()
	response, err := client.TextToVoiceWithContext(ctx, request)
	if err != nil {
		logTencentTTSError(err)
		return err
	}
	if response == nil || response.Response == nil || response.Response.Audio == nil {
		return errors.New("Tencent TTS response did not include audio")
	}
	audio, err := base64.StdEncoding.DecodeString(*response.Response.Audio)
	if err != nil {
		return fmt.Errorf("Tencent TTS audio base64 decode failed: %w", err)
	}
	if len(audio) == 0 {
		return errors.New("Tencent TTS returned empty audio")
	}
	return playPCM16k(robot, audio)
}

func loadTencentTTSConfig() (tencentTTSConfig, error) {
	vars.ApplyTTSDefaults()
	cfg := tencentTTSConfig{
		SecretID:   strings.TrimSpace(os.Getenv("TENCENTCLOUD_SECRET_ID")),
		SecretKey:  strings.TrimSpace(os.Getenv("TENCENTCLOUD_SECRET_KEY")),
		Region:     strings.TrimSpace(vars.APIConfig.TTS.TencentRegion),
		VoiceType:  vars.APIConfig.TTS.TencentVoiceType,
		SampleRate: vars.APIConfig.TTS.TencentSampleRate,
		Codec:      strings.ToLower(strings.TrimSpace(vars.APIConfig.TTS.TencentCodec)),
		Speed:      vars.APIConfig.TTS.TencentSpeed,
		Volume:     vars.APIConfig.TTS.TencentVolume,
		Timeout:    time.Duration(vars.APIConfig.TTS.TencentTimeoutSec) * time.Second,
	}
	if cfg.SecretID == "" {
		return cfg, errors.New("TENCENTCLOUD_SECRET_ID is required for Tencent TTS")
	}
	if cfg.SecretKey == "" {
		return cfg, errors.New("TENCENTCLOUD_SECRET_KEY is required for Tencent TTS")
	}
	if cfg.Region == "" {
		cfg.Region = "ap-guangzhou"
	}
	if cfg.VoiceType == 0 {
		cfg.VoiceType = 601009
	}
	if cfg.SampleRate == 0 {
		cfg.SampleRate = 16000
	}
	if cfg.Codec == "" {
		cfg.Codec = "pcm"
	}
	if cfg.Codec != "pcm" {
		return cfg, errors.New("Tencent TTS codec must be pcm for Vector playback")
	}
	if cfg.SampleRate != 16000 {
		return cfg, errors.New("Tencent TTS sample rate must be 16000 for Vector playback")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 15 * time.Second
	}
	return cfg, nil
}

func buildTencentTTSRequest(cfg tencentTTSConfig, input string) (*tts.TextToVoiceRequest, error) {
	request := tts.NewTextToVoiceRequest()
	payload := map[string]interface{}{
		"Text":            input,
		"SessionId":       "wirepod-" + uuid.NewString(),
		"Volume":          cfg.Volume,
		"Speed":           cfg.Speed,
		"ProjectId":       0,
		"ModelType":       1,
		"VoiceType":       cfg.VoiceType,
		"PrimaryLanguage": 1,
		"SampleRate":      cfg.SampleRate,
		"Codec":           cfg.Codec,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	if err := request.FromJsonString(string(payloadBytes)); err != nil {
		return nil, err
	}
	return request, nil
}

func logTencentTTSError(err error) {
	if sdkErr, ok := err.(*tcerrors.TencentCloudSDKError); ok {
		logger.Println("Tencent TTS error: RequestId=" + sdkErr.GetRequestId() + " Code=" + sdkErr.GetCode() + " Message=" + sdkErr.GetMessage())
		return
	}
	logger.Println("Tencent TTS error: " + err.Error())
}
