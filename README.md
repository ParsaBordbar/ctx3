<p align="center">
  <img width="200" alt="ctx3" src="https://github.com/user-attachments/assets/7cca9bd3-5587-4df0-a7c1-c5b4323d6a8e" />
</p>

# Context Tree (CTX3)

Context Tree (ctx3) is a free, open source, tool, written in go that helps you and your favorite LLM to understand the code base better, by providing data about the structure of the code-base and its dependencies.

# What Can It do!?

ctx3 is a combination of two basic Ideas,  file tree cli tool that can help LLMs to understand the file hierarchy (Tree) and a brief over view of files (Context) !
so simply its a cli tool that gives you meta data about your code-base via commands!

### File Tree 
Prints Files Hierarchy, shows the project Structure

### Context 
Provides Meta Data and summaries of the Dependencies and instructions of the code. Out puts a json-prompt that helps LLMs 

## How To Install

For using ctx3 you need to have **Go** installed so if you don't have it consider installing it [from here.](https://go.dev/doc/install) 

Then you can install it with Go like this:

```bash
go install github.com/parsabordbar/ctx3@latest
```

## How To Use

All your files and folders are presented as a tree in the file explorer. You can switch from one to another by clicking a file in the tree.

to see the file tree use:
```
ctx3 print <path>
```
if no path is provided it defaults to the current directory.

- -\- help => to see instructions

to get the summary you can use:
```
ctx3 context <path>
```
if no path is provided it defaults to the current directory.
you can use wild cards:

- -\-json OR -j => to outputs json prompt
- \-\-help OR -h => see instructions 
