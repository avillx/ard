package fstools

import (
	"ard/internal/agent"
	"ard/internal/files"
	"ard/internal/prompt"
	"ard/internal/tools"
	"ard/internal/types"
	"bytes"
	"errors"
)

type ConsolidationInstuctFS struct {
	agent.ToolServer
}

func NewConsolidationInstuctFS(storage files.FileStorage, skipPatterns []string) (*ConsolidationInstuctFS, error) {
	ts, err := NewRawFileSystemToolServer(storage, skipPatterns)
	if err != nil {
		return nil, err
	}
	return &ConsolidationInstuctFS{
		ToolServer: ts,
	}, nil
}

func (r *ConsolidationInstuctFS) Instruction() string {
	return prompt.ConsolidationFSInstruction()
}

type FileSystemToolServer struct {
	*tools.BuildInToolServer

	storage files.FileStorage
}

func NewFileSystemToolServer(storage files.FileStorage, skipPatterns []string) (*FileSystemToolServer, error) {
	ts, err := NewRawFileSystemToolServer(storage, skipPatterns)
	if err != nil {
		return nil, err
	}

	return &FileSystemToolServer{
		storage:           storage,
		BuildInToolServer: ts,
	}, nil
}

func NewRawFileSystemToolServer(storage files.FileStorage, skipPatterns []string) (*tools.BuildInToolServer, error) {
	findTool, err := NewFindTool(storage, skipPatterns)
	if err != nil {
		return nil, err
	}

	readTool, err := NewReadTool(storage, skipPatterns)
	if err != nil {
		return nil, err
	}

	return tools.NewBuildInToolServer(
		&EditTool{storage: storage},
		&MoveTool{storage: storage},
		readTool,
		findTool,
		&WriteTool{storage: storage},
	), nil
}

func (r *FileSystemToolServer) Instruction() string {
	return prompt.FileSystemInstruction()
}

func mapErrs(err error) error {
	if errors.Is(err, types.ErrIsNotExist) {
		return types.NewAgentMistakeError("path is not found, ensure path existence")
	}
	return err
}

func isBinary(data []byte) bool {
	return bytes.IndexByte(data, nullByte) != -1
}

const nullByte = 0x00
