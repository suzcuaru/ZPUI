package app

// Автостарт движка при запуске приложения (прод-поведение).
// Переопределяется флагом -zapret-autostart=false (SetDevNoZapretAutostart).
var devNoZapretAutostart = false

// SetDevNoZapretAutostart — вызывается из main при флаге -zapret-autostart=false.
func SetDevNoZapretAutostart(v bool) { devNoZapretAutostart = v }
