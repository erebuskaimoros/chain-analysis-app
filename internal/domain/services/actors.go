package services

import (
	"context"

	"chain-analysis-app/internal/app"
)

type ActorService struct {
	legacy *app.App
}

func (s *ActorService) List(ctx context.Context) ([]app.Actor, error) {
	return s.legacy.ListActors(ctx)
}

func (s *ActorService) Upsert(ctx context.Context, actorID int64, req app.ActorUpsertRequest) (app.Actor, error) {
	return s.legacy.UpsertActor(ctx, actorID, req)
}

func (s *ActorService) Delete(ctx context.Context, actorID int64) error {
	return s.legacy.DeleteActor(ctx, actorID)
}

type AnnotationService struct {
	legacy *app.App
}

func (s *AnnotationService) ListAnnotations(ctx context.Context) ([]app.AddressAnnotation, error) {
	return s.legacy.ListAddressAnnotations(ctx)
}

func (s *AnnotationService) UpsertAnnotation(ctx context.Context, address, kind, value string) error {
	return s.legacy.UpsertAddressAnnotation(ctx, address, kind, value)
}

func (s *AnnotationService) DeleteAnnotation(ctx context.Context, address, kind string) error {
	return s.legacy.DeleteAddressAnnotation(ctx, address, kind)
}

func (s *AnnotationService) ListBlocklist(ctx context.Context) ([]app.BlocklistedAddress, error) {
	return s.legacy.ListBlocklistedAddresses(ctx)
}

func (s *AnnotationService) AddToBlocklist(ctx context.Context, address, reason string) error {
	return s.legacy.AddToBlocklist(ctx, address, reason)
}

func (s *AnnotationService) RemoveFromBlocklist(ctx context.Context, address string) error {
	return s.legacy.RemoveFromBlocklist(ctx, address)
}
