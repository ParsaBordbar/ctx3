package pack

import (
	"bytes"
	"context"
	"fmt"
)

// Pack walks the repository and renders the output into a single buffer.
// Every OutputFormat has a renderer; the section layout is identical across
// them, only the delimiters differ.
func Pack(ctx context.Context, cfg Config) ([]byte, Report, error) {
	structure, filesFn, err := renderersFor(cfg.OutputFormat)
	if err != nil {
		return nil, Report{}, err
	}

	files, tree, rep, err := WalkAndCollect(ctx, cfg)
	if err != nil {
		return nil, rep, err
	}

	var buf bytes.Buffer
	if cfg.Sections.Structure {
		structure(&buf, tree, cfg)
	}
	if cfg.Sections.Files {
		filesFn(&buf, files, cfg)
	}
	return buf.Bytes(), rep, nil
}

type (
	structureRenderer func(*bytes.Buffer, *dirNode, Config)
	filesRenderer     func(*bytes.Buffer, []FileEntry, Config)
)

// renderersFor resolves a format to its pair of section renderers. It runs
// before the walk so an unknown format fails immediately instead of after
// reading the whole repository.
func renderersFor(f OutputFormat) (structureRenderer, filesRenderer, error) {
	switch f {
	case FormatXML:
		return renderXMLStructure, renderXMLFiles, nil
	case FormatMD:
		return renderMDStructure, renderMDFiles, nil
	case FormatTXT:
		return renderTXTStructure, renderTXTFiles, nil
	default:
		return nil, nil, fmt.Errorf("unsupported format: %s (want: xml, md, txt)", f)
	}
}
