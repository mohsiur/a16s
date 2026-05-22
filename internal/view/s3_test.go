package view

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	s3Types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	kindpkg "github.com/mohsiur/a16s/internal/view/kind"
)

func TestS3KindName(t *testing.T) {
	k := &s3Kind{}
	if k.Name() != "s3" {
		t.Fatalf("Name = %q; want %q", k.Name(), "s3")
	}
}

func TestS3KindSelectionRoundTrip(t *testing.T) {
	k := &s3Kind{}
	b := &s3Types.Bucket{Name: aws.String("logs-bucket")}
	k.SetSelection(b)
	if k.Selection() != b {
		t.Fatalf("Selection round-trip failed")
	}
}

func TestS3KindResetClearsSelection(t *testing.T) {
	k := &s3Kind{}
	k.SetSelection(&s3Types.Bucket{Name: aws.String("x")})
	k.Reset()
	if k.Selection() != nil {
		t.Fatalf("Selection after Reset = %v; want nil", k.Selection())
	}
}

func TestS3KindRegistered(t *testing.T) {
	k, ok := kindpkg.Get("s3")
	if !ok {
		t.Fatal("s3 kind not registered")
	}
	if _, ok := k.(*s3Kind); !ok {
		t.Fatalf("registered kind is %T; want *s3Kind", k)
	}
}
