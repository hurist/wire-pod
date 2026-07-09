package wirepod_ttr

import (
	"context"
	"time"

	"github.com/fforchino/vector-go-sdk/pkg/vector"
	"github.com/fforchino/vector-go-sdk/pkg/vectorpb"
)

func chunkPCM16k(pcm []byte) [][]byte {
	var audioChunks [][]byte
	for len(pcm) > 0 {
		if len(pcm) < 1024 {
			chunk := make([]byte, 1024)
			copy(chunk, pcm)
			audioChunks = append(audioChunks, chunk)
			break
		}
		audioChunks = append(audioChunks, pcm[:1024])
		pcm = pcm[1024:]
	}
	return audioChunks
}

func playPCM16k(robot *vector.Vector, pcm []byte) error {
	return playPCM16kChunks(robot, chunkPCM16k(pcm))
}

func playPCM16kChunks(robot *vector.Vector, audioChunks [][]byte) error {
	if len(audioChunks) == 0 {
		return nil
	}
	vclient, err := robot.Conn.ExternalAudioStreamPlayback(context.Background())
	if err != nil {
		return err
	}
	if err := vclient.Send(&vectorpb.ExternalAudioStreamRequest{
		AudioRequestType: &vectorpb.ExternalAudioStreamRequest_AudioStreamPrepare{
			AudioStreamPrepare: &vectorpb.ExternalAudioStreamPrepare{
				AudioFrameRate: 16000,
				AudioVolume:    100,
			},
		},
	}); err != nil {
		return err
	}

	var chunksToDetermineLength []byte
	for _, chunk := range audioChunks {
		chunksToDetermineLength = append(chunksToDetermineLength, chunk...)
	}
	go func() {
		for _, chunk := range audioChunks {
			vclient.Send(&vectorpb.ExternalAudioStreamRequest{
				AudioRequestType: &vectorpb.ExternalAudioStreamRequest_AudioStreamChunk{
					AudioStreamChunk: &vectorpb.ExternalAudioStreamChunk{
						AudioChunkSizeBytes: uint32(len(chunk)),
						AudioChunkSamples:   chunk,
					},
				},
			})
			time.Sleep(time.Millisecond * 25)
		}
		vclient.Send(&vectorpb.ExternalAudioStreamRequest{
			AudioRequestType: &vectorpb.ExternalAudioStreamRequest_AudioStreamComplete{
				AudioStreamComplete: &vectorpb.ExternalAudioStreamComplete{},
			},
		})
	}()
	time.Sleep(pcmLength(chunksToDetermineLength) + (time.Millisecond * 50))
	return nil
}
