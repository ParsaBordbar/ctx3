package functions

import (
	"os"
	"path/filepath"
	"strings"
)

type ParameterType string

const (
	Untyped ParameterType = ""
	Any     ParameterType = "any"
)

type Parameter struct {
	Name string        `json:"name"`
	Type ParameterType `json:"type"`
}

type ReturnType struct {
	Type        ParameterType `json:"type"`
	IsArray     bool          `json:"is_array,omitempty"`
	IsOptional  bool          `json:"is_optional,omitempty"`
	IsNullable  bool          `json:"is_nullable,omitempty"`
}

type Function struct {
	Name        string        `json:"name"`
	Language    string        `json:"language"`
	Path        string        `json:"path"`
	LineNumber  int           `json:"line_number"`
	Parameters  []Parameter   `json:"parameters"`
	ReturnTypes []ReturnType  `json:"return_types"`
	IsAsync     bool          `json:"is_async,omitempty"`
	IsExported  bool          `json:"is_exported,omitempty"`
	Decorators  []string      `json:"decorators,omitempty"`
	DocString   string        `json:"doc_string,omitempty"`
	Signature   string        `json:"signature"`
}

type FunctionContext struct {
	Functions    []Function `json:"functions"`
	TotalFunctions int       `json:"total_functions"`
	LanguageStats map[string]int `json:"language_stats"`
}

func AnalyzeFunctions(rootDir string) FunctionContext {
	ctx := FunctionContext{
		Functions:    []Function{},
		LanguageStats: make(map[string]int),
	}

	filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		if shouldSkipFile(path) {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		var functions []Function

		switch ext {
		case ".go":
			functions = analyzeGoFile(path)
		case ".py":
			functions = analyzePythonFile(path)
		case ".js", ".jsx":
			functions = analyzeJavaScriptFile(path)
		case ".ts", ".tsx":
			functions = analyzeTypeScriptFile(path)
		case ".rs":
			functions = analyzeRustFile(path)
		case ".c", ".h":
			functions = analyzeCFile(path)
		}

		for i := range functions {
			functions[i].Path = GetRlativePath(rootDir, path)
			ctx.Functions = append(ctx.Functions, functions[i])
			ctx.LanguageStats[functions[i].Language]++
		}

		return nil
	})

	ctx.TotalFunctions = len(ctx.Functions)
	return ctx
}

//TODO 
func analyzeCFile(path string) []Function {
	panic("unimplemented")
}

func analyzeRustFile(path string) []Function {
	panic("unimplemented")
}

func analyzeTypeScriptFile(path string) []Function {
	panic("unimplemented")
}

func analyzeJavaScriptFile(path string) []Function {
	panic("unimplemented")
}

func analyzePythonFile(path string) []Function {
	panic("unimplemented")
}

func analyzeGoFile(path string) []Function {
	panic("unimplemented")
}

func shouldSkipFile(path string) bool {
	panic("unimplemented")
}