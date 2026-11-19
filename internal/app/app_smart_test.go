package app

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"zpui/internal/database"
)

// Секция data.strategies собирается только из протестированных стратегий
// текущего оператора; непротестированные (av<0) и чужие игровые баллы
// не отправляются.
func TestSyncStrategiesSectionFilters(t *testing.T) {
	const isp = "ut-smart-isp"
	if err := database.SetCurrentOperator(isp, "Тест-оператор", ""); err != nil {
		t.Fatalf("set current operator: %v", err)
	}
	if err := database.SaveOperatorStrategy(isp, "AlphaSmart.bat", `[{"avg_ms":100,"yandex_ping_ms":120,"yandex_speed_mbps":12.5}]`); err != nil {
		t.Fatalf("save strategy: %v", err)
	}
	if err := database.UpdateStrategyAvailability(isp, "AlphaSmart.bat", 87.5); err != nil {
		t.Fatalf("update availability: %v", err)
	}
	if err := database.SaveOperatorStrategy(isp, "Untested.bat", ""); err != nil {
		t.Fatalf("save strategy: %v", err)
	}
	if err := database.SaveStrategyGame(database.StrategyGame{
		Strategy: "AlphaSmart.bat", Score: 0.8, PingMs: 95, Ok: 4, Total: 5, TestedAt: time.Now(),
	}); err != nil {
		t.Fatalf("save game: %v", err)
	}
	if err := database.SaveStrategyGame(database.StrategyGame{
		Strategy: "ForeignGame.bat", Score: 0.9, PingMs: 90, Ok: 4, Total: 5, TestedAt: time.Now(),
	}); err != nil {
		t.Fatalf("save game: %v", err)
	}

	sec := (&App{}).sectionStrategies()
	raw, err := json.Marshal(sec)
	if err != nil {
		t.Fatal(err)
	}
	frag := string(raw)
	if !strings.Contains(frag, "AlphaSmart.bat") {
		t.Fatalf("strategies missing: %q", frag)
	}
	if strings.Contains(frag, "Untested.bat") {
		t.Fatalf("untested strategy must not be sent: %q", frag)
	}
	if !strings.Contains(frag, "0.8") {
		t.Fatalf("games missing: %q", frag)
	}
	if strings.Contains(frag, "ForeignGame.bat") {
		t.Fatalf("game score of non-tested strategy must not be sent: %q", frag)
	}
}

// Ответ strategies.top разбирается клиентом: имена/баллы доходят до смарт-ветки.
func TestSyncStrategiesTopParse(t *testing.T) {
	doc := `{"operator":"Тест-оператор","source":"device",
		"items":[{"name":"General.bat","score":71.2,"av":87.5,"ping":120,"speed":12.3,"samples":9,"last_sample":""}],
		"games":[{"name":"General.bat","score":0.8,"ping":95,"samples":4}],
		"plan":[{"name":"General.bat"}]}`
	var st syncStrategiesTop
	if err := json.Unmarshal([]byte(doc), &st); err != nil {
		t.Fatal(err)
	}
	if st.Operator != "Тест-оператор" || len(st.Items) != 1 || len(st.Games) != 1 {
		t.Fatalf("parsed: %+v", st)
	}
	if st.Items[0].Name != "General.bat" || st.Items[0].Score != 71.2 || st.Items[0].Samples != 9 {
		t.Fatalf("item: %+v", st.Items[0])
	}
	if st.Games[0].Score != 0.8 || st.Games[0].PingMs != 95 {
		t.Fatalf("game: %+v", st.Games[0])
	}
	if len(st.Plan) != 1 || st.Plan[0].Name != "General.bat" {
		t.Fatalf("plan: %+v", st.Plan)
	}
}
