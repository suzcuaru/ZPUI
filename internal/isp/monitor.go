package isp

import (
	"fmt"
	"time"

	"zpui/internal/database"
)

type OperatorChange struct {
	OldKey  string
	OldName string
	NewKey  string
	NewName string
	Op      *Operator
}

type LogFunc func(category, msg string)

type Monitor struct {
	log           LogFunc
	checkInterval time.Duration
	onChange      func(OperatorChange)
	stopCh        chan struct{}
	currentKey    string
}

func NewMonitor(log LogFunc, checkInterval time.Duration, onChange func(OperatorChange)) *Monitor {
	return &Monitor{
		log:           log,
		checkInterval: checkInterval,
		onChange:      onChange,
		stopCh:        make(chan struct{}),
	}
}

func (m *Monitor) Start() {
	m.detectAndSave()

	ticker := time.NewTicker(m.checkInterval)
	defer ticker.Stop()
	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.detectAndSave()
		}
	}
}

func (m *Monitor) Stop() {
	close(m.stopCh)
}

func (m *Monitor) detectAndSave() {
	op, err := DetectOperator()
	if err != nil {
		m.log("isp", "detect failed: "+err.Error())
		return
	}

	now := time.Now()
	err = database.UpsertOperatorInfo(&database.OperatorInfo{
		Key:       op.Key,
		Name:      op.Name,
		ISP:       op.ISP,
		ASN:       op.ASN,
		City:      op.City,
		Org:       op.Org,
		FirstSeen: now,
		LastSeen:  now,
	})
	if err != nil {
		m.log("isp", "upsert operator info failed: "+err.Error())
	}

	cur, _ := database.GetCurrentOperator()
	oldKey := ""
	oldName := ""
	if cur != nil {
		oldKey = cur.OperatorKey
		oldName = cur.OperatorName
	}

	database.SetCurrentOperator(op.Key, op.Name, "")

	if oldKey != "" && oldKey != op.Key {
		m.log("isp", fmt.Sprintf("operator changed: %s -> %s", oldName, op.Name))
		if m.onChange != nil {
			m.onChange(OperatorChange{
				OldKey:  oldKey,
				OldName: oldName,
				NewKey:  op.Key,
				NewName: op.Name,
				Op:      op,
			})
		}
	} else if oldKey == "" {
		m.log("isp", "operator detected: "+op.Name)
		if m.onChange != nil {
			m.onChange(OperatorChange{
				OldKey:  "",
				OldName: "",
				NewKey:  op.Key,
				NewName: op.Name,
				Op:      op,
			})
		}
	}

	m.currentKey = op.Key
}

func (m *Monitor) GetCurrentKey() string {
	return m.currentKey
}

func DetectOnce() (*Operator, error) {
	return DetectOperator()
}
