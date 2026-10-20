package domain_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	domain "gooze.dev/pkg/gooze/internal/domain"
	domainmocks "gooze.dev/pkg/gooze/internal/domain/mocks"
	m "gooze.dev/pkg/gooze/internal/model"
)

func TestReportPublisher_Push(t *testing.T) {
	reg := domainmocks.NewMockOCIRegistry(t)
	reg.On("Push", mock.Anything, "ref:tag", m.Path("/r"), domain.RegistryOptions{PlainHTTP: true}).Return(nil)

	pub := domain.NewReportPublisher(reg)

	require.NoError(t, pub.Push(context.Background(), domain.PushArgs{Reports: "/r", Ref: "ref:tag", PlainHTTP: true}))
	reg.AssertExpectations(t)
}

func TestReportPublisher_Pull(t *testing.T) {
	reg := domainmocks.NewMockOCIRegistry(t)
	reg.On("Pull", mock.Anything, "ref:tag", m.Path("/r"), domain.RegistryOptions{Insecure: true}).Return(nil)

	pub := domain.NewReportPublisher(reg)

	require.NoError(t, pub.Pull(context.Background(), domain.PullArgs{Reports: "/r", Ref: "ref:tag", Insecure: true}))
	reg.AssertExpectations(t)
}

func TestReportPublisher_Validation(t *testing.T) {
	pub := domain.NewReportPublisher(domainmocks.NewMockOCIRegistry(t))

	require.Error(t, pub.Push(context.Background(), domain.PushArgs{Reports: "/r"})) // missing ref
	require.Error(t, pub.Push(context.Background(), domain.PushArgs{Ref: "x"}))      // missing reports
	require.Error(t, pub.Pull(context.Background(), domain.PullArgs{Reports: "/r"})) // missing ref
}

func TestReportPublisher_PropagatesError(t *testing.T) {
	reg := domainmocks.NewMockOCIRegistry(t)
	boom := errors.New("boom")
	reg.On("Push", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(boom)

	pub := domain.NewReportPublisher(reg)

	err := pub.Push(context.Background(), domain.PushArgs{Reports: "/r", Ref: "x"})
	require.ErrorIs(t, err, boom)
}
