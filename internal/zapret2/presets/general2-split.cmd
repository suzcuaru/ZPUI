@echo off
chcp 65001 >nul
start "zapret2: http,https (split)" /min "%~dp0binaries\winws2.exe" ^
--wf-tcp-out=80,443 ^
--lua-init=@"%~dp0lua\zapret-lib.lua" --lua-init=@"%~dp0lua\zapret-antidpi.lua" ^
--lua-init="fake_default_tls = tls_mod(fake_default_tls,'rnd,rndsni')" ^
--filter-tcp=80 --filter-l7=http ^
  --out-range=-d10 ^
  --payload=http_req ^
   --lua-desync=fakedsplit:ip_autottl=-2,3-20:ip6_autottl=-2,3-20:tcp_md5 ^
  --new ^
--filter-tcp=443 --filter-l7=tls ^
  --out-range=-d10 ^
  --payload=tls_client_hello ^
   --lua-desync=fake:blob=fake_default_tls:tcp_md5:repeats=11:tls_mod=rnd,dupsid ^
   --lua-desync=multidisorder:pos=1,midsld ^
  --new ^
--filter-tcp=443 --filter-l7=tls ^
  --out-range=-d10 ^
  --payload=tls_client_hello ^
   --lua-desync=fakedsplit:ip_autottl=-2,3-20:ip6_autottl=-2,3-20:tcp_md5