package executor

import (
	"fmt"
	"os"
)

func (e *Executor) PersistStreamEventSnapshot(taskID string, streamRaw []byte) (*StreamEventSnapshot, error) {
	artifacts := e.ArtifactPaths(taskID)
	if err := os.WriteFile(artifacts.StreamFile, streamRaw, 0o644); err != nil {
		return nil, fmt.Errorf("写入 stream-json 原始流失败: %w", err)
	}
	events, err := ParseStreamJSONL(streamRaw)
	if err != nil {
		return nil, err
	}
	snapshot := AggregateStreamEvents(taskID, events)
	if err := writeStreamEventSnapshotFile(artifacts.EventFile, snapshot); err != nil {
		return nil, err
	}
	return snapshot, nil
}

func (e *Executor) LoadStreamEventSnapshot(taskID string) (*StreamEventSnapshot, error) {
	artifacts := e.ArtifactPaths(taskID)
	snapshot, err := readStreamEventSnapshotFile(artifacts.EventFile)
	if err != nil {
		return nil, err
	}
	if snapshot != nil {
		return snapshot, nil
	}
	streamRaw, err := os.ReadFile(artifacts.StreamFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("读取 stream-json 原始流失败: %w", err)
	}
	events, err := ParseStreamJSONL(streamRaw)
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, nil
	}
	snapshot = AggregateStreamEvents(taskID, events)
	if err := writeStreamEventSnapshotFile(artifacts.EventFile, snapshot); err != nil {
		return nil, err
	}
	return snapshot, nil
}

func readStreamEventSnapshotFile(path string) (*StreamEventSnapshot, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("读取 stream 事件快照失败: %w", err)
	}
	snapshot, err := UnmarshalStreamEventSnapshot(payload)
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

func writeStreamEventSnapshotFile(path string, snapshot *StreamEventSnapshot) error {
	payload, err := MarshalStreamEventSnapshot(snapshot)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		return fmt.Errorf("写入 stream 事件快照失败: %w", err)
	}
	return nil
}
