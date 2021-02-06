package azdeployer

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"text/template"
	"time"
)

// ArchiveBuilder is functions.yml
type ArchiveBuilder struct {
	Files      []string `json:"files"`
	Functions  []string `json:"functions"`
	Executable string   `json:"executable"`
}

// BuildArchive builds a zip file
func (b *ArchiveBuilder) BuildArchive(zipFile io.Writer) (err error) {
	zipWriter := zip.NewWriter(zipFile)
	defer func() {
		closeErr := zipWriter.Close()
		if err == nil {
			err = closeErr
		}
	}()

	for _, file := range b.Files {
		err = addFileToZip(zipWriter, file)
		if err != nil {
			return err
		}
	}

	for _, fName := range b.Functions {
		err = addToZip(zipWriter, filepath.Join(fName, "function.json"), []byte(functionJSON))
		if err != nil {
			return err
		}
	}

	hostJSON, err := buildHostFile(b.Executable)
	if err != nil {
		return err
	}

	return addToZip(zipWriter, "host.json", hostJSON)
}

func addToZip(zw *zip.Writer, name string, content []byte) error {
	w, err := zw.CreateHeader(&zip.FileHeader{
		Name:     name,
		Method:   zip.Deflate,
		Modified: time.Now().UTC(),
	})
	if err != nil {
		return err
	}
	_, err = w.Write(content)
	return err
}

func addFileToZip(zipWriter *zip.Writer, filename string) (err error) {
	fileToZip, err := os.Open(filename) //nolint:gosec // checked
	if err != nil {
		return err
	}

	defer func() {
		closeErr := fileToZip.Close()
		if err == nil {
			err = closeErr
		}
	}()

	// Get the file information
	info, err := fileToZip.Stat()
	if err != nil {
		return err
	}

	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}

	// Using FileInfoHeader() above only uses the basename of the file. If we want
	// to preserve the folder structure we can overwrite this with the full path.
	header.Name = filename

	// Change to deflate to gain better compression
	// see http://golang.org/pkg/archive/zip/#pkg-constants
	header.Method = zip.Deflate

	writer, err := zipWriter.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = io.Copy(writer, fileToZip)
	return err
}

func buildHostFile(executable string) ([]byte, error) {
	tmpl := template.Must(template.New("").Parse(`{
  "version": "2.0",
  "logging": {
    "applicationInsights": {
      "samplingSettings": {
        "isEnabled": true,
        "excludedTypes": "Request"
      }
    }
  },
  "extensionBundle": {
    "id": "Microsoft.Azure.Functions.ExtensionBundle",
    "version": "[1.*, 2.0.0)"
  },
  "customHandler": {
    "description": {
      "defaultExecutablePath": "{{ .Executable }}",
      "workingDirectory": "",
      "arguments": []
    },
    "enableForwardingHttpRequest": true
  }
}
`))
	var buf bytes.Buffer
	err := tmpl.Execute(&buf, map[string]string{
		"Executable": executable,
	})
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

const functionJSON = `{
  "bindings": [
    {
      "authLevel": "anonymous",
      "type": "httpTrigger",
      "direction": "in",
      "name": "req",
      "methods": [
        "get",
        "post"
      ]
    },
    {
      "type": "http",
      "direction": "out",
      "name": "res"
    }
  ]
}
`
