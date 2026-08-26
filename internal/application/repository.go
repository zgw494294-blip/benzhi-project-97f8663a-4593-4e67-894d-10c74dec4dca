package application

import (
	"context"

	"seed-vigor-workbench/internal/domain"
)

type Repository interface {
	Create(context.Context, *domain.GerminationAssay, string) error
	Get(context.Context, string) (*domain.GerminationAssay, error)
	List(context.Context) ([]domain.GerminationAssay, error)
	Update(context.Context, string, int64, string, string, map[string]any, func(*domain.GerminationAssay) error) (*domain.GerminationAssay, error)
}
