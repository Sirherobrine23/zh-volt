//go:build !static

package page

import (
	"io/fs"
	"os"
)

var source fs.FS = os.DirFS("./web/page/web_src")
