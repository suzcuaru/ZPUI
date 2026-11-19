package database

import "testing"

// MarkObservedStrategy — «наблюдение» без истории: is_active ставится,
// use_count/applied_at не трогаются. На этом держится правило стартового
// подбора: чистая система должна иметь пустую базу оператора (полный
// перебор), пока стратегию реально не применили или не прогнали тест.
func TestMarkObservedStrategyDoesNotCreateHistory(t *testing.T) {
	op := uniq("op-obs")
	if err := EnsureOperatorStrategiesBatch(op, []string{"a.bat", "b.bat"}, []string{"A", "B"}); err != nil {
		t.Fatalf("ensure batch: %v", err)
	}

	if err := MarkObservedStrategy(op, "a.bat"); err != nil {
		t.Fatalf("mark observed: %v", err)
	}
	stored, err := GetOperatorStrategies(op)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	var a, b *OperatorStrategy
	for i := range stored {
		switch stored[i].Strategy {
		case "a.bat":
			a = &stored[i]
		case "b.bat":
			b = &stored[i]
		}
	}
	if a == nil || b == nil {
		t.Fatalf("rows missing: %+v", stored)
	}
	if !a.IsActive {
		t.Fatalf("observed strategy must be active")
	}
	if b.IsActive {
		t.Fatalf("other strategies must be deactivated")
	}
	if a.UseCount != 0 || a.AppliedAt != nil || a.TestedAt != nil {
		t.Fatalf("observed mark must not create history: %+v", a)
	}

	// Реальное применение — история появляется, и база перестаёт быть «пустой».
	if err := MarkActiveStrategy(op, "a.bat", "manual"); err != nil {
		t.Fatalf("mark active: %v", err)
	}
	stored, err = GetOperatorStrategies(op)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	for _, s := range stored {
		if s.Strategy == "a.bat" && (s.UseCount == 0 || s.AppliedAt == nil) {
			t.Fatalf("applied strategy must carry history: %+v", s)
		}
	}
}

// ClearOperatorStrategyEra — эра ручного прогона: стратегии вне keep теряют
// данные проверок (JSON/tested_at/pct), история применений сохраняется.
func TestClearOperatorStrategyEra(t *testing.T) {
	op := uniq("op-era")
	if err := EnsureOperatorStrategiesBatch(op, []string{"keep.bat", "old.bat"}, []string{"K", "O"}); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if err := SaveOperatorStrategy(op, "keep.bat", `[{"strategy":"keep.bat","ok":8,"total":10}]`); err != nil {
		t.Fatalf("save keep: %v", err)
	}
	if err := UpdateStrategyAvailability(op, "keep.bat", 80); err != nil {
		t.Fatalf("pct keep: %v", err)
	}
	if err := SaveOperatorStrategy(op, "old.bat", `[{"strategy":"old.bat","ok":3,"total":10}]`); err != nil {
		t.Fatalf("save old: %v", err)
	}
	if err := UpdateStrategyAvailability(op, "old.bat", 30); err != nil {
		t.Fatalf("pct old: %v", err)
	}
	if err := MarkActiveStrategy(op, "old.bat", "manual"); err != nil {
		t.Fatalf("mark active: %v", err)
	}

	if err := ClearOperatorStrategyEra(op, []string{"keep.bat"}); err != nil {
		t.Fatalf("clear era: %v", err)
	}
	stored, err := GetOperatorStrategies(op)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	byName := map[string]OperatorStrategy{}
	for _, s := range stored {
		byName[s.Strategy] = s
	}
	keep, ok := byName["keep.bat"]
	if !ok || keep.AutoTestResults == "" || keep.TestedAt == nil || keep.AvailabilityPct != 80 {
		t.Fatalf("keep row must keep era data: %+v", keep)
	}
	old, ok := byName["old.bat"]
	if !ok {
		t.Fatalf("old row missing")
	}
	if old.AutoTestResults != "" || old.TestedAt != nil || old.AvailabilityPct != -1 {
		t.Fatalf("old row must be wiped: %+v", old)
	}
	if old.UseCount != 1 || old.AppliedAt == nil || !old.IsActive || old.LastSource != "manual" {
		t.Fatalf("apply history must survive wipe: %+v", old)
	}

	// Пустой keep — стираются данные всего оператора.
	if err := ClearOperatorStrategyEra(op, nil); err != nil {
		t.Fatalf("clear era all: %v", err)
	}
	stored, err = GetOperatorStrategies(op)
	if err != nil {
		t.Fatalf("get after full wipe: %v", err)
	}
	for _, s := range stored {
		if s.AutoTestResults != "" || s.TestedAt != nil || s.AvailabilityPct != -1 {
			t.Fatalf("all rows must be wiped: %+v", s)
		}
	}
}
