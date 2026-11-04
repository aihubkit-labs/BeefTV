package buildinfo

import "encoding/base64"

// Optional distribution identity. Defaults preserve the official local build.
// Display names may change; ProductID and DataDirectory remain stable per brand.
var (
	ProductName       = "BeefTV"
	ProductScheme     = "beeftv"
	DataDirectory     = "BeefTV"
	ProductNameBase64 string
)

func init() {
	if ProductNameBase64 != "" {
		value, err := base64.StdEncoding.DecodeString(ProductNameBase64)
		if err != nil || len(value) == 0 {
			panic("invalid distribution product name")
		}
		ProductName = string(value)
	}
}
