package rest

import (
	"fmt"
	"net/http"
	"strings"

	"vraxel.io/vraxel/lib/runtime"
)

// FileResponse is a special response type for file downloads.
// When returned from a HandlerFunc, the framework writes raw bytes
// with Content-Disposition instead of JSON/YAML serialization.
type FileResponse struct {
	runtime.TypeMeta
	FileName    string
	ContentType string
	Data        []byte
}

func (f *FileResponse) GetTypeMeta() *runtime.TypeMeta { return &f.TypeMeta }

// WriteFile is the exported form of writeFileResponse for use by the v2
// apiserver handler wrappers (lib/apiserver), sharing the exact
// Content-Disposition semantics with the v1 handlers.
func WriteFile(w http.ResponseWriter, statusCode int, fr *FileResponse) {
	writeFileResponse(w, statusCode, fr)
}

// writeFileResponse writes a FileResponse to the HTTP response writer.
func writeFileResponse(w http.ResponseWriter, statusCode int, fr *FileResponse) {
	// Sanitize filename to prevent header injection
	safeName := strings.NewReplacer(`"`, "", "\r", "", "\n", "").Replace(fr.FileName)
	w.Header().Set("Content-Type", fr.ContentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, safeName))
	w.WriteHeader(statusCode)
	_, _ = w.Write(fr.Data)
}
