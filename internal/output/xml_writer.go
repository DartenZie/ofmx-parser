// Package output validates and serializes custom XML output.
//
// Author: Miroslav Pašek
package output

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"os"

	"github.com/DartenZie/ofmx-parser/internal/domain"
)

type XMLWriter interface {
	Write(ctx context.Context, doc domain.OutputDocument, path string) error
}

// XMLFileWriter writes XML documents to files.
type XMLFileWriter struct{}

// Write serializes a domain output document to XML and writes it to path.
func (w XMLFileWriter) Write(_ context.Context, doc domain.OutputDocument, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return domain.NewError(domain.ErrOutput, fmt.Sprintf("failed to create output file %q", path), err)
	}
	defer f.Close()

	buf := bytes.NewBufferString(xml.Header)
	enc := xml.NewEncoder(buf)
	enc.Indent("", "  ")

	if err := enc.Encode(doc); err != nil {
		return domain.NewError(domain.ErrOutput, "failed to encode output XML", err)
	}

	if err := enc.Flush(); err != nil {
		return domain.NewError(domain.ErrOutput, "failed to flush XML encoder", err)
	}

	if _, err := f.Write(buf.Bytes()); err != nil {
		return domain.NewError(domain.ErrOutput, fmt.Sprintf("failed to write output file %q", path), err)
	}

	return nil
}
