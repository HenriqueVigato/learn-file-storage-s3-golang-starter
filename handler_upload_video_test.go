package main

import (
	"testing"
)

func TestGetVidoAspectRation(t *testing.T) {
	aspectRatin, err := getVideoAspectRation("./samples/boots-video-horizontal.mp4")
	if err != nil {
		t.Errorf("Nao era esperado um erro aqui %v", err)
	}

	if aspectRatin != "16:9" {
		t.Errorf("aspect ration diferent(%s) from expected(%s)", aspectRatin, "16:9")
	}
}
