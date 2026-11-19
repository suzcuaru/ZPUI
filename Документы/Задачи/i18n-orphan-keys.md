# i18n: ключи-сироты в локали

> **Статус: выполнено 04.11.2026 (рефакторинг-батч).** Удалено 601 ключ из
> ru.json+en.json синхронно (2068 → 1235 листьев). Список ниже НЕ удалён
> целиком: скан из 03.11 был всеведущ не до конца — например, группа
> `auth.err.*` строится динамически (`t('auth.err.' + res.error)` в
> ProfileDrawer), как и `zapret.*`, `health.problem.*`, `startup.steps.*`,
> `startup.stage.*`, `syncd.*`, `dns.provider.*`, `docs.sections/groups`,
> `feedback.topic.*`, `logs.*`, `toast.title*`. Перед удалением каждое семейство
> проверено на конкатенацию/шаблоны; 85 «ложных сирот» оставлены. Список ниже —
> историческая справка.

Дата: 03.11.2026. Методика: листовые ключи ru.json, не встречающиеся в web/src (и dev-стендах) литерально; ключи под динамическими префиксами (t(`seg.` / t('seg.' + x)) исключены. **Перед удалением сверить вручную** — скан консервативен, но не всеведущ (косвенное построение ключей, ключи из бэкенда, future-код).

Всего кандидатов: **682** из 2103 листовых ключей ru.json. Без пары в en.json: 0.

## auth (60)

- `auth.titleLogin`
- `auth.codeSite.desc`
- `auth.codeSite.field`
- `auth.codeSite.connect`
- `auth.codeSite.note`
- `auth.br.title`
- `auth.br.codeNotActive`
- `auth.br.doneDevice`
- `auth.br.desc`
- `auth.br.open`
- `auth.br.openFailed`
- `auth.br.waiting`
- `auth.br.codeLabel`
- `auth.br.waitFor`
- `auth.br.reopen`
- `auth.br.expired`
- `auth.br.doneTitle`
- `auth.br.done`
- `auth.br.doneHint`
- `auth.br.note`
- `auth.err.badNick`
- `auth.err.bad_request`
- `auth.err.bad_login`
- `auth.err.bad_email`
- `auth.err.login_taken`
- `auth.err.email_taken`
- `auth.err.invalid_credentials`
- `auth.err.account_banned`
- `auth.err.bad_code`
- `auth.err.captcha_required`
- `auth.err.captcha_invalid`
- `auth.err.code_unknown_or_expired`
- `auth.err.too_many_attempts`
- `auth.err.mail_not_configured`
- `auth.err.mail_send_failed`
- `auth.err.renew_window`
- `auth.err.subscription_permanent`
- `auth.err.device_limit`
- `auth.err.email_required`
- `auth.err.bad_to`
- `auth.err.current_device`
- `auth.err.not_found`
- `auth.err.server_unreachable`
- `auth.err.timeout`
- `auth.err.connection_refused`
- `auth.err.not_authorized`
- `auth.err.email_disabled`
- `auth.err.bad_totp`
- `auth.err.bad_captcha`
- `auth.err.bad_recovery`
- `auth.err.bad_token`
- `auth.err.expired_token`
- `auth.err.rate_limited`
- `auth.err.device_bound`
- `auth.err.forbidden`
- `auth.err.nick_taken`
- `auth.err.totp_setup_required`
- `auth.err.internal`
- `auth.err.bad_promo`
- `auth.err.device_owner`

## banner (1)

- `banner.dismiss`

## checker (4)

- `checker.viaBypass`
- `checker.proxyOff`
- `checker.dnsMismatch`
- `checker.bypassWorks`

## common (22)

- `common.apply`
- `common.understand`
- `common.update`
- `common.updating`
- `common.start`
- `common.stop`
- `common.wait`
- `common.downloaded`
- `common.downloading`
- `common.clear`
- `common.skip`
- `common.notFound`
- `common.disable`
- `common.rows`
- `common.version`
- `common.inDevelopment`
- `common.next`
- `common.back`
- `common.yes`
- `common.no`
- `common.done`
- `common.continue`

## crash (2)

- `crash.title`
- `crash.reload`

## dashboard (12)

- `dashboard.title`
- `dashboard.components`
- `dashboard.allStable`
- `dashboard.system`
- `dashboard.traffic`
- `dashboard.totalColon`
- `dashboard.zapretSettings`
- `dashboard.upToDate`
- `dashboard.moreSuffix`
- `dashboard.actions`
- `dashboard.strategy`
- `dashboard.provider`

## dns (13)

- `dns.provider.xbox`
- `dns.provider.yandex`
- `dns.provider.cloudflare`
- `dns.provider.google`
- `dns.provider.adguard`
- `dns.provider.comss`
- `dns.customDesc`
- `dns.stateXbox`
- `dns.stateMixed`
- `dns.applyNote`
- `dns.health.ok`
- `dns.health.fail`
- `dns.health.unknown`

## footer (5)

- `footer.total`
- `footer.up`
- `footer.down`
- `footer.serverOnline`
- `footer.serverOffline`

## fpc (6)

- `fpc.title`
- `fpc.desc`
- `fpc.current`
- `fpc.next`
- `fpc.nextRepeat`
- `fpc.submit`

## header (5)

- `header.proxy`
- `header.proxyStarted`
- `header.proxyStopped`
- `header.xboxDnsOn`
- `header.xboxDnsOff`

## health (7)

- `health.title`
- `health.problem.zapret_missing.title`
- `health.problem.zapret_missing.desc`
- `health.action.openDns`
- `health.action.restart`
- `health.action.autoselect`
- `health.action.diagnostics`

## monitor (5)

- `monitor.download`
- `monitor.upload`
- `monitor.netSpeed`
- `monitor.netPing`
- `monitor.noDevices`

## nav (5)

- `nav.sync`
- `nav.plans`
- `nav.sysinfo`
- `nav.user`
- `nav.tunnel`

## offline (2)

- `offline.connecting`
- `offline.retry`

## plans (26)

- `plans.title`
- `plans.currentTitle`
- `plans.authNeeded`
- `plans.login`
- `plans.active`
- `plans.expired`
- `plans.expiredMeta`
- `plans.daysLeft`
- `plans.permanent`
- `plans.trialBadge`
- `plans.trialHint`
- `plans.yourPlan`
- `plans.renew`
- `plans.renewNotYet`
- `plans.free`
- `plans.perMonth`
- `plans.devices`
- `plans.speedTotal`
- `plans.speedPerDevice`
- `plans.unlimited`
- `plans.payBoosty`
- `plans.payOpening`
- `plans.payOpened`
- `plans.baseNote`
- `plans.payHow`
- `plans.loadError`

## powerPaused (1)

- `powerPaused.syncTitle`

## profile (32)

- `profile.defaultName`
- `profile.authTitle`
- `profile.authHint`
- `profile.copyCode`
- `profile.botCopied`
- `profile.codeTtl`
- `profile.stepAllow`
- `profile.searchHint`
- `profile.waiting`
- `profile.subTrial`
- `profile.subExpired`
- `profile.codeWaiting`
- `profile.codeTapCopy`
- `profile.stepBotPlain`
- `profile.stepSend`
- `profile.openAccount`
- `profile.openFailed`
- `profile.thisPC`
- `profile.fldCode`
- `profile.expiredOn`
- `profile.kvDevices`
- `profile.byDefault`
- `profile.renewNote`
- `profile.upsellHint`
- `profile.upsellBtn`
- `profile.kvEmail`
- `profile.tunnelReset`
- `profile.devicesTitle`
- `profile.subsTitle`
- `profile.subsSmart`
- `profile.tunnelAddedHint`
- `profile.secSub`

## recovery (1)

- `recovery.modalTitle`

## serverOffline (5)

- `serverOffline.title`
- `serverOffline.desc`
- `serverOffline.retry`
- `serverOffline.hide`
- `serverOffline.support`

## setm (15)

- `setm.title`
- `setm.nick`
- `setm.currentNick`
- `setm.newNickPh`
- `setm.change`
- `setm.password`
- `setm.passHint`
- `setm.devices`
- `setm.noDevices`
- `setm.devicesNote`
- `setm.unbind`
- `setm.deviceUnbound`
- `setm.nickDone`
- `setm.passDone`
- `setm.yourEmail`

## settings (49)

- `settings.title`
- `settings.vpnIgnore`
- `settings.vpnIgnoreDesc`
- `settings.vpnIgnoreEmpty`
- `settings.vpnIgnoreRemove`
- `settings.soundsScheme`
- `settings.soundScheme_standard`
- `settings.soundScheme_soft`
- `settings.soundScheme_quiet`
- `settings.prevLang`
- `settings.nextLang`
- `settings.notifications`
- `settings.notifyStrategyTest`
- `settings.notifZpuiUpdates`
- `settings.notifZpuiUpdatesDesc`
- `settings.notifZapretUpdates`
- `settings.notifZapretUpdatesDesc`
- `settings.notifMissingFiles`
- `settings.notifMissingFilesDesc`
- `settings.notifServiceStatusDesc`
- `settings.notifResourceDrop`
- `settings.notifResourceDropDesc`
- `settings.threshold`
- `settings.zapretSection`
- `settings.localInstall`
- `settings.status`
- `settings.satellites`
- `settings.updateStarted`
- `settings.errorPrefix`
- `settings.componentUpdateStarted`
- `settings.removeServiceDesc`
- `settings.svcRowReason`
- `settings.svcName`
- `settings.svcCrashes`
- `settings.notifErrors`
- `settings.notifServiceCrash`
- `settings.fullReinstallDesc`
- `settings.diagnostics`
- `settings.tgProxyCard`
- `settings.tgProxyUpdWithZpui`
- `settings.checkForUpdates`
- `settings.serverTitle`
- `settings.serverUrl`
- `settings.serverKey`
- `settings.serverActivate`
- `settings.serverState`
- `settings.serverTunnel`
- `settings.server_activated`
- `settings.server_notActivated`

## setup (44)

- `setup.boot`
- `setup.selfcheck.checking`
- `setup.selfcheck.repairing`
- `setup.selfcheck.ok`
- `setup.envscan.scanning`
- `setup.envscan.foundThirdParty`
- `setup.envscan.foundVpn`
- `setup.envscan.clean`
- `setup.download.downloading`
- `setup.download.verifying`
- `setup.firstrun.title`
- `setup.firstrun.desc`
- `setup.firstrun.setup`
- `setup.firstrun.skip`
- `setup.test.title`
- `setup.test.baseline`
- `setup.test.control`
- `setup.test.testing`
- `setup.test.autoSelect`
- `setup.test.waiting`
- `setup.test.done`
- `setup.results.title`
- `setup.results.desc`
- `setup.results.applied`
- `setup.results.best`
- `setup.results.apply`
- `setup.results.blocked`
- `setup.results.allOk`
- `setup.results.noViable`
- `setup.config.title`
- `setup.config.gameFilter`
- `setup.config.gameFilterDesc`
- `setup.config.ipset`
- `setup.config.customResources`
- `setup.config.customResourcesPh`
- `setup.config.dns`
- `setup.config.dnsDesc`
- `setup.config.proxy`
- `setup.config.proxyDesc`
- `setup.config.notifications`
- `setup.config.autostart`
- `setup.config.apply`
- `setup.done.title`
- `setup.done.desc`

## sidebar (15)

- `sidebar.dashboardTip`
- `sidebar.zapretTip`
- `sidebar.proxyTip`
- `sidebar.dnsTip`
- `sidebar.monitorTip`
- `sidebar.syncTip`
- `sidebar.settingsTip`
- `sidebar.healthTip`
- `sidebar.autoSelectTip`
- `sidebar.checkerTip`
- `sidebar.logsTip`
- `sidebar.helpTip`
- `sidebar.reportLabel`
- `sidebar.powerSavingDeepTip`
- `sidebar.userTip`

## stand (4)

- `stand.subtitle`
- `stand.closeToast`
- `stand.themeToggle`
- `stand.titlebarMinimize`

## startup (39)

- `startup.loadingSystem`
- `startup.steps.connect`
- `startup.steps.health`
- `startup.steps.envscan`
- `startup.steps.check-service`
- `startup.steps.decide-service`
- `startup.steps.remove-service`
- `startup.steps.check-local`
- `startup.steps.install`
- `startup.steps.install-service`
- `startup.steps.strategy-prompt`
- `startup.steps.auto-strategy`
- `startup.steps.auto-select`
- `startup.steps.sync-suggest`
- `startup.downloaded`
- `startup.cloning`
- `startup.serviceConflict.title`
- `startup.serviceConflict.desc`
- `startup.serviceConflict.remove`
- `startup.serviceConflict.workWithout`
- `startup.serviceConflict.skipHint`
- `startup.zapretInstalled.title`
- `startup.zapretInstalled.version`
- `startup.zapretInstalled.strategy`
- `startup.zapretInstalled.desc`
- `startup.zapretInstalled.selectStrategy`
- `startup.serviceHealth.checking`
- `startup.serviceHealth.retrying`
- `startup.serviceHealth.reinstalling`
- `startup.syncSuggest.title`
- `startup.syncSuggest.desc`
- `startup.syncSuggest.skip`
- `startup.syncSuggest.pinDesc`
- `startup.syncSuggest.searching`
- `startup.syncSuggest.searchingDesc`
- `startup.stage.prep`
- `startup.stage.check`
- `startup.stage.install`
- `startup.stage.select`

## status (4)

- `status.notInstalled`
- `status.updateAvailable`
- `status.problem`
- `status.broken`

## sub (14)

- `sub.title`
- `sub.current`
- `sub.renew`
- `sub.pay`
- `sub.devices`
- `sub.renewWindowHint`
- `sub.testCheckout`
- `sub.testNote`
- `sub.plan`
- `sub.amount`
- `sub.months`
- `sub.completeTestPay`
- `sub.paidDone`
- `sub.paidDoneNote`

## sync (40)

- `sync.title`
- `sync.desc`
- `sync.devices`
- `sync.paired`
- `sync.pairedDesc`
- `sync.available`
- `sync.availableDesc`
- `sync.noDevices`
- `sync.noPaired`
- `sync.disabledHint`
- `sync.deviceName`
- `sync.deviceNameDesc`
- `sync.trusted`
- `sync.offline`
- `sync.lastSync`
- `sync.never`
- `sync.sync`
- `sync.syncNow`
- `sync.syncing`
- `sync.connect`
- `sync.disconnect`
- `sync.pinRequested`
- `sync.enterPinTitle`
- `sync.enterPinDesc`
- `sync.yourPinTitle`
- `sync.yourPinDesc`
- `sync.yourPinHint`
- `sync.unknownPeer`
- `sync.confirm`
- `sync.syncDone`
- `sync.syncFailed`
- `sync.renameTitle`
- `sync.renameDesc`
- `sync.renameAccept`
- `sync.renameDecline`
- `sync.renameAccepted`
- `sync.renameDeclined`
- `sync.roleMaster`
- `sync.roleSlave`
- `sync.remoteUnpaired`

## syncd (13)

- `syncd.sec.status`
- `syncd.sec.runtime`
- `syncd.sec.availability`
- `syncd.sec.network`
- `syncd.sec.strategies`
- `syncd.sec.traffic`
- `syncd.sec.operator`
- `syncd.sec.errors`
- `syncd.sec.events`
- `syncd.sec.system`
- `syncd.sec.versions`
- `syncd.sec.config`
- `syncd.sec.exclusions`

## sysinfo (14)

- `sysinfo.title`
- `sysinfo.loading`
- `sysinfo.refreshing`
- `sysinfo.refresh`
- `sysinfo.externalAddress`
- `sysinfo.publicIp`
- `sysinfo.isp`
- `sysinfo.org`
- `sysinfo.geolocation`
- `sysinfo.city`
- `sysinfo.country`
- `sysinfo.localAddresses`
- `sysinfo.notFound`
- `sysinfo.copied`

## themeEditor (3)

- `themeEditor.fit_cover`
- `themeEditor.fit_contain`
- `themeEditor.fit_repeat`

## themeGallery (1)

- `themeGallery.saved`

## toast (7)

- `toast.updateAvailable`
- `toast.errorInZpui`
- `toast.zapretStartFail`
- `toast.titleSuccess`
- `toast.titleError`
- `toast.titleWarning`
- `toast.titleInfo`

## tunnel (46)

- `tunnel.until`
- `tunnel.trafficLabel`
- `tunnel.trafficPeriod`
- `tunnel.speedLabel`
- `tunnel.speedValue`
- `tunnel.speedUnlimited`
- `tunnel.devicesLabel`
- `tunnel.devicesOf`
- `tunnel.onlineShort`
- `tunnel.devOnline`
- `tunnel.devOffline`
- `tunnel.devVpnTag`
- `tunnel.noSub`
- `tunnel.permanentSub`
- `tunnel.zapretBlockTitle`
- `tunnel.zapretBlockText`
- `tunnel.zapretBlockBtn`
- `tunnel.zapretPaused`
- `tunnel.title`
- `tunnel.onFull`
- `tunnel.onLists`
- `tunnel.connecting`
- `tunnel.off`
- `tunnel.errActive`
- `tunnel.gateAuthTitle`
- `tunnel.gateAuthText`
- `tunnel.gateAuthBtn`
- `tunnel.gateSubTitle`
- `tunnel.gateSubText`
- `tunnel.gateExpiredTitle`
- `tunnel.gateExpiredText`
- `tunnel.gateExpiredBtn`
- `tunnel.gateNoVpnTitle`
- `tunnel.gateNoVpnText`
- `tunnel.errSub`
- `tunnel.errNoVpn`
- `tunnel.scopeTitle`
- `tunnel.modeFull`
- `tunnel.modeFullHint`
- `tunnel.modeLists`
- `tunnel.modeListsHint`
- `tunnel.zapretNote`
- `tunnel.handshake`
- `tunnel.daysLeft`
- `tunnel.subTitle`
- `tunnel.slotFree`

## update (3)

- `update.installNow`
- `update.details`
- `update.currentVersion`

## user (26)

- `user.password`
- `user.passwordRepeat`
- `user.logout`
- `user.noDevices`
- `user.err.invalid_email`
- `user.err.email_taken`
- `user.err.too_often`
- `user.err.bad_code`
- `user.err.invalid_input`
- `user.err.bad_credentials`
- `user.err.passwordMismatch`
- `user.err.key_already_bound`
- `user.err.subscription_required`
- `user.err.no_free_slots`
- `user.err.key_not_set`
- `user.err.bad_key`
- `user.err.unknown`
- `user.err.invite_required`
- `user.err.bad_invite`
- `user.err.invite_used`
- `user.err.invite_revoked`
- `user.err.registration_disabled`
- `user.err.login_taken`
- `user.err.device_limit`
- `user.err.rate_limited`
- `user.unbind`

## user2 (48)

- `user2.statusNone`
- `user2.vpnOff`
- `user2.vpnConnecting`
- `user2.vpnUp`
- `user2.vpnError`
- `user2.vpnLocked`
- `user2.vpnLockedHint`
- `user2.vpnLockedPlans`
- `user2.vpnIdle`
- `user2.traffic`
- `user2.ofGb`
- `user2.speed`
- `user2.mbps`
- `user2.speedPerDevice`
- `user2.speedUnlimited`
- `user2.handshake`
- `user2.priority`
- `user2.priorityTip`
- `user2.passTitle`
- `user2.passChanged`
- `user2.planName`
- `user2.modeTitle`
- `user2.mode.off`
- `user2.mode.full`
- `user2.mode.lists`
- `user2.modeListsHint`
- `user2.accTitle`
- `user2.accEmail`
- `user2.accLogin`
- `user2.accRegistered`
- `user2.accDays`
- `user2.accStatus`
- `user2.accStatusActive`
- `user2.accStatusExpiring`
- `user2.accStatusExpired`
- `user2.accStatusUnlimited`
- `user2.liveTabs`
- `user2.tabVpn`
- `user2.tabTg`
- `user2.manualTitle`
- `user2.manualHint`
- `user2.manualEndpoint`
- `user2.manualPubkey`
- `user2.manualIp`
- `user2.manualDns`
- `user2.liveNotInSub`
- `user2.tgNotEnabled`
- `user2.tgNoLink`

## userOffline (2)

- `userOffline.title`
- `userOffline.desc`

## user_new (34)

- `user_new.loginLabel`
- `user_new.loginPlaceholder`
- `user_new.loginShort`
- `user_new.passShort`
- `user_new.activatedOk`
- `user_new.subscription`
- `user_new.validUntil`
- `user_new.kpiUntil`
- `user_new.subExpired`
- `user_new.availableInProduct`
- `user_new.fingerprint`
- `user_new.fpHint`
- `user_new.boundDate`
- `user_new.limitReached`
- `user_new.limitReason`
- `user_new.unbindTitle`
- `user_new.unbindConfirm`
- `user_new.modeLogin`
- `user_new.modeRegister`
- `user_new.loginTitle`
- `user_new.loginSub`
- `user_new.regTitle`
- `user_new.regSub`
- `user_new.submitLogin`
- `user_new.submitRegister`
- `user_new.loginOk`
- `user_new.planGo`
- `user_new.planLite`
- `user_new.planPro`
- `user_new.planExcelsior`
- `user_new.authTagline`
- `user_new.authPoint1`
- `user_new.authPoint2`
- `user_new.authPoint3`

## user_st (13)

- `user_st.devicesMetric`
- `user_st.devicesOf`
- `user_st.bothOnline`
- `user_st.oneOnline`
- `user_st.loadError`
- `user_st.retry`
- `user_st.profile`
- `user_st.loggingOut`
- `user_st.thisDevice`
- `user_st.connected`
- `user_st.osLabel`
- `user_st.zpuiVersion`
- `user_st.statusLabel`

## xboxdns (34)

- `xboxdns.internalDns`
- `xboxdns.runningDesc`
- `xboxdns.stoppedDesc`
- `xboxdns.dnsEnabled`
- `xboxdns.dnsDisabled`
- `xboxdns.dnsServers`
- `xboxdns.primaryDns`
- `xboxdns.secondaryDns`
- `xboxdns.serverInfo`
- `xboxdns.serverInfoDesc`
- `xboxdns.healthOk`
- `xboxdns.healthFail`
- `xboxdns.healthUnknown`
- `xboxdns.healthCheckedAt`
- `xboxdns.serviceInfo`
- `xboxdns.stdAddrs`
- `xboxdns.encAddrs`
- `xboxdns.primaryV4`
- `xboxdns.secondaryV4`
- `xboxdns.dotLabel`
- `xboxdns.dohLabel`
- `xboxdns.editAddrs`
- `xboxdns.editAddrsDesc`
- `xboxdns.invalidIp`
- `xboxdns.listSaved`
- `xboxdns.thisComputer`
- `xboxdns.lanIp`
- `xboxdns.lanIpDesc`
- `xboxdns.setupGuide`
- `xboxdns.step1`
- `xboxdns.step2`
- `xboxdns.step3`
- `xboxdns.step4`
- `xboxdns.step5`

