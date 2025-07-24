package util

import (
	"github.com/omec-project/openapi/models"
)

func CompareExtSnssai(a, b models.Snssai) bool {
	// Compare Sst and Sd fields (add more fields if needed)
	if a.Sst != b.Sst {
		return false
	}
	if a.Sd != b.Sd {
		return false
	}
	// If there are other comparable fields, add them here.
	return true
}
