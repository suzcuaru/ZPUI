package tgproxy

import (
	"strconv"

	"zpui/internal/config"
)

// buildArgs переводит TelegramProxyConfig в аргументы CLI ZPUI-TgProxy.exe
// (родной headless-вход апстрима proxy/tg_ws_proxy.py, его Docker-контракт).
func buildArgs(pc config.TelegramProxyConfig, logPath string) []string {
	args := []string{
		"--host", pc.BindHost,
		"--port", strconv.Itoa(pc.Port),
		"--secret", pc.Secret,
		"--log-file", logPath,
		"--log-max-mb", "5",
		"--log-backups", "1",
	}
	if pc.PoolSize > 0 {
		args = append(args, "--pool-size", strconv.Itoa(pc.PoolSize))
	}
	if pc.BufKB > 0 {
		args = append(args, "--buf-kb", strconv.Itoa(pc.BufKB))
	}
	if !pc.FallbackCF {
		args = append(args, "--no-cfproxy")
	}
	if pc.NoH2 {
		args = append(args, "--no-h2")
	}
	if pc.NoSecure {
		args = append(args, "--no-secure")
	}
	if pc.ProxyProtocol {
		args = append(args, "--proxy-protocol")
	}
	if pc.ForceTestDC {
		args = append(args, "--force-test-dc")
	}
	if pc.FakeTLSDomain != "" {
		args = append(args, "--fake-tls-domain", pc.FakeTLSDomain)
	}
	for _, d := range pc.DCIP {
		args = append(args, "--dc-ip", d)
	}
	for _, d := range pc.CFUserDomains {
		args = append(args, "--cfproxy-domain", d)
	}
	for _, d := range pc.CFWorkerDomains {
		args = append(args, "--cfproxy-worker-domain", d)
	}
	if pc.Verbose {
		args = append(args, "-v")
	}
	return args
}
