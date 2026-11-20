//go:build integration

package delivery_test

import (
	"errors"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

func isCode(err error, c apperrors.Code) bool {
	var ae *apperrors.Error
	return errors.As(err, &ae) && ae.Code == c
}
