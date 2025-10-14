package pack

import (
	"bytes"
	"context"
	"fmt"
)

// Pack walks the repository and renders the output into a single buffer.
// Currently supports XML (sample-style). MD/TXT can be added later.
func Pack(ctx context.Context, cfg Config) ([]byte, Report, error) {
	files, tree, rep, err := WalkAndCollect(ctx, cfg)
	if err != nil {
		return nil, rep, err
	}

	var buf bytes.Buffer
	switch cfg.OutputFormat {
	case FormatXML:
		if cfg.Sections.Structure {
			renderXMLStructure(&buf, tree, cfg)
		}
		if cfg.Sections.Files {
			renderXMLFiles(&buf, files, cfg)
		}
	default:
		return nil, rep, fmt.Errorf("unsupported format: %s (only xml is implemented)", cfg.OutputFormat)
	}

	return buf.Bytes(), rep, nil
}
