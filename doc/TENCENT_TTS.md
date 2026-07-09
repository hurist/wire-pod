# Tencent Cloud TTS

Wire-Pod now supports Tencent Cloud TTS as the default text-to-speech provider for arbitrary robot speech. This path is used by LLM replies, `KGSim`, behavior-control speech helpers, and the SDK App `/api-sdk/say_text` endpoint.

## Runtime Flow

```text
text
  -> Tencent Cloud TextToVoice
  -> base64 PCM audio
  -> raw PCM 16 kHz / 16-bit / mono
  -> Vector ExternalAudioStreamPlayback
  -> speaker playback
```

Tencent TTS is requested with `Codec=pcm` and `SampleRate=16000`, so the returned audio can be sent directly to Vector without MP3/WAV decoding or resampling.

## Required Credentials

Tencent TTS reuses the existing Tencent Cloud credential environment variables:

```bash
export TENCENTCLOUD_SECRET_ID="your-secret-id"
export TENCENTCLOUD_SECRET_KEY="your-secret-key"
```

The credentials must have access to Tencent Cloud TTS. If credentials are missing or invalid, Wire-Pod logs the Tencent TTS error and falls back to Vector's built-in voice so speech does not fail completely.

## Default Configuration

`apiConfig.json` contains the TTS settings:

```json
{
  "tts": {
    "provider": "tencent",
    "tencent_region": "ap-guangzhou",
    "tencent_voice_type": 601009,
    "tencent_sample_rate": 16000,
    "tencent_codec": "pcm",
    "tencent_speed": 0,
    "tencent_volume": 0,
    "tencent_timeout_seconds": 15
  }
}
```

Defaults are applied automatically when older config files do not contain the `tts` block.

## Environment Variables

These variables are used when creating a new config or filling missing TTS values:

| Variable | Default | Description |
|---|---:|---|
| `TTS_PROVIDER` | `tencent` | `tencent`, `openai`, or `vector` |
| `TENCENT_TTS_REGION` | `ap-guangzhou` | Tencent TTS API region |
| `TENCENT_TTS_VOICE_TYPE` | `601009` | Tencent voice type ID |
| `TENCENT_TTS_SAMPLE_RATE` | `16000` | Must stay `16000` for Vector playback |
| `TENCENT_TTS_CODEC` | `pcm` | Must stay `pcm` for Vector playback |
| `TENCENT_TTS_SPEED` | `0` | Tencent speed value |
| `TENCENT_TTS_VOLUME` | `0` | Tencent volume value |
| `TENCENT_TTS_TIMEOUT_SECONDS` | `15` | Request timeout |

Docker supports the same settings with `WIREPOD_` prefixes, for example:

```bash
WIREPOD_TTS_PROVIDER=tencent
WIREPOD_TENCENT_TTS_VOICE_TYPE=601009
```

## Web UI

Open the Server Settings page and use the TTS Setup section. The provider defaults to Tencent Cloud TTS. The current UI exposes provider, region, voice type, speed, and volume.

## Troubleshooting

| Symptom | Likely Cause | Fix |
|---|---|---|
| Robot uses English Vector voice | Tencent credentials missing or request failed | Check logs and set `TENCENTCLOUD_SECRET_ID` / `TENCENTCLOUD_SECRET_KEY` |
| Tencent error `AuthFailure.InvalidAuthorization` | Invalid credential or signature | Regenerate Tencent Cloud API keys |
| Tencent error `InvalidParameterValue.VoiceType` | Voice type unavailable | Choose a valid Tencent voice type |
| No audio or distorted audio | Codec/sample rate changed | Keep `tencent_codec=pcm` and `tencent_sample_rate=16000` |
| Tencent error `UnsupportedOperation.AccountArrears` | Account billing issue | Check Tencent Cloud billing and TTS service status |
