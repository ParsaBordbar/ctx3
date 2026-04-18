# ctx3  
Fast, intelligent repo‑analysis tooling for developers and AI agents.

[![Go Version](https://img.shields.io/badge/Go-1.23+-00ADD8?logo=go)](https://go.dev/)
[![Build](https://img.shields.io/github/actions/workflow/status/parsabordbar/ctx3/ci.yml?label=build)](https://github.com/parsabordbar/ctx3/actions)
[![Downloads](https://img.shields.io/github/downloads/parsabordbar/ctx3/total.svg)](https://github.com/parsabordbar/ctx3/releases)
[![License](https://img.shields.io/github/license/parsabordbar/ctx3)](LICENSE)
[![Stars](https://img.shields.io/github/stars/parsabordbar/ctx3?style=social)](https://github.com/parsabordbar/ctx3)

`ctx3` helps you instantly understand any codebase: structure, metadata, languages, dependencies, and fully packed AI‑ready bundles.  
Lightning‑fast. Zero configuration. Perfect for developers, LLM workflows, and automated agents.

---

## What It Does  
- Print clean project trees  
- Analyze file metadata + detect languages  
- Show language/file‑type percentages  
- Extract entry points & (future) dependency graphs  
- Pack entire repos into a single LLM‑friendly artifact  
- Outputs in JSON, TOON (LLM‑optimized), XML, Markdown, or plain text

---

## Commands

### `ctx3 context <path>`
Quick metadata overview.  
Token‑efficient modes for LLMs (JSON, TOON).

### `ctx3 print <path>`
Beautiful directory tree output.

### `ctx3 percentage <path>`
Language breakdown in clean ASCII graphs.

### `ctx3 pack <path>`
Bundle entire repos into a single AI‑friendly artifact (structure + file contents).  
Highly configurable with include/ignore globs, redaction, concurrency, formats, and size limits.

---

## 📦 Install
```bash
go install github.com/<your-username>/ctx3@latest
```

## Coming Soon
Database diagrams (SQL → charts)
Deeper dependency inspection
Smarter language detection
Extended binary handling

## 🤝 Contribute
PRs and feature requests are welcome!
