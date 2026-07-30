(function(ZPUI) {
  var h = ZPUI.h;
  var useState = ZPUI.useState;

  var DNS = {
    primary4: '111.88.96.50',
    secondary4: '111.88.96.51',
    primary6: '2a00:ab00:1233:26::50',
    secondary6: '2a00:ab00:1233:26::51',
    dot: 'xbox-dns.ru',
    doh: 'https://xbox-dns.ru/dns-query',
  };

  var PLATFORMS = [
    {
      id: 'windows', label: 'ПК (Windows)',
      steps: [
        'Откройте Параметры → Сеть и Интернет → Wi-Fi или Ethernet.',
        'Нажмите Свойства оборудования.',
        'В разделе Назначение DNS-сервера нажмите Изменить.',
        'Выберите Вручную, включите IPv4 и введите DNS адреса. В меню шифрования выберите Только шифрование (DNS через HTTPS).',
        'В поле DoH введите: ' + DNS.doh,
      ],
    },
    {
      id: 'xbox', label: 'Xbox',
      steps: [
        'Нажмите кнопку Xbox на геймпаде, откройте Настройки → Общие → Сетевые настройки.',
        'Выберите Дополнительные настройки → DNS-серверы.',
        'Выберите Вручную и введите адреса: ' + DNS.primary4 + ' и ' + DNS.secondary4,
      ],
    },
    {
      id: 'ps', label: 'PlayStation',
      steps: [
        'Откройте Настройки → Сеть → Настройка подключения к интернету.',
        'Выберите подключение, нажмите Дополнительные настройки.',
        'Установите DNS-серверы → Вручную и введите адреса: ' + DNS.primary4 + ' и ' + DNS.secondary4,
      ],
    },
    {
      id: 'android', label: 'Android',
      steps: [
        'Откройте Настройки → Wi-Fi.',
        'Нажмите и удерживайте вашу сеть, выберите Изменить сеть.',
        'Разверните Дополнительные настройки → Настройка IP → Статический.',
        'Введите DNS: ' + DNS.primary4 + ' и ' + DNS.secondary4,
      ],
    },
    {
      id: 'ios', label: 'iOS',
      steps: [
        'Откройте Настройки → Wi-Fi.',
        'Нажмите на (i) рядом с вашей сетью.',
        'Прокрутите вниз до Настроить DNS → Вручную.',
        'Добавьте серверы: ' + DNS.primary4 + ' и ' + DNS.secondary4,
      ],
    },
    {
      id: 'router', label: 'Роутер',
      steps: [
        'Откройте веб-интерфейс роутера (192.168.1.1 или 192.168.0.1).',
        'Найдите раздел WAN / Интернет / DHCP.',
        'В поле DNS-сервер введите: ' + DNS.primary4 + ' и ' + DNS.secondary4,
        'Сохраните и перезагрузите роутер.',
      ],
    },
    {
      id: 'browser', label: 'Браузер (DoH)',
      steps: [
        'Включите DoH в настройках браузера.',
        'Chrome/Edge: Настройки → Конфиденциальность → Безопасность → Использовать безопасный DNS.',
        'Firefox: Настройки → Приватность → DNS через HTTPS.',
        'Введите провайдера: ' + DNS.doh,
      ],
    },
  ];

  function XboxDnsPage(ctx) {
    var tab = useState('windows');
    var setTab = tab[1];
    var active = tab[0];
    var platform = null;
    for (var i = 0; i < PLATFORMS.length; i++) {
      if (PLATFORMS[i].id === active) { platform = PLATFORMS[i]; break; }
    }

    return h('div', null,
      h('div', { className: 'page-title-row' },
        h('span', { className: 'page-title' }, 'Xbox DNS — настройка')
      ),
      h('div', { className: 'xdns-hero' },
        h('div', { className: 'xdns-hero-text' },
          'Начните пользоваться за 2 минуты. Выберите платформу и следуйте инструкции.'
        )
      ),
      h('div', { className: 'xdns-servers' },
        h('div', { className: 'xdns-server-group' },
          h('span', { className: 'xdns-server-label' }, 'Основной (IPv4)'),
          h('code', { className: 'xdns-copy', onClick: function() { navigator.clipboard.writeText(DNS.primary4); } }, DNS.primary4)
        ),
        h('div', { className: 'xdns-server-group' },
          h('span', { className: 'xdns-server-label' }, 'Дополнительный (IPv4)'),
          h('code', { className: 'xdns-copy', onClick: function() { navigator.clipboard.writeText(DNS.secondary4); } }, DNS.secondary4)
        ),
        h('div', { className: 'xdns-server-group' },
          h('span', { className: 'xdns-server-label' }, 'Основной (IPv6)'),
          h('code', { className: 'xdns-copy', onClick: function() { navigator.clipboard.writeText(DNS.primary6); } }, DNS.primary6)
        ),
        h('div', { className: 'xdns-server-group' },
          h('span', { className: 'xdns-server-label' }, 'Дополнительный (IPv6)'),
          h('code', { className: 'xdns-copy', onClick: function() { navigator.clipboard.writeText(DNS.secondary6); } }, DNS.secondary6)
        ),
        h('div', { className: 'xdns-server-group' },
          h('span', { className: 'xdns-server-label' }, 'DNS-over-TLS'),
          h('code', { className: 'xdns-copy', onClick: function() { navigator.clipboard.writeText(DNS.dot); } }, DNS.dot)
        ),
        h('div', { className: 'xdns-server-group' },
          h('span', { className: 'xdns-server-label' }, 'DNS-over-HTTPS'),
          h('code', { className: 'xdns-copy', onClick: function() { navigator.clipboard.writeText(DNS.doh); } }, DNS.doh)
        )
      ),
      h('div', { className: 'xdns-tabs' },
        PLATFORMS.map(function(p) {
          return h('button', {
            key: p.id,
            className: 'xdns-tab' + (active === p.id ? ' active' : ''),
            onClick: function() { setTab(p.id); },
          }, p.label);
        })
      ),
      h('div', { className: 'xdns-platform' },
        platform
          ? h('ol', { className: 'xdns-steps' },
              platform.steps.map(function(s, i) {
                return h('li', { key: i }, s);
              })
            )
          : null
      )
    );
  }

  ZPUI.registerPage('xbox-dns', {
    label: 'Xbox DNS',
    icon: h('svg', { viewBox: '0 0 24 24', width: '24', height: '24', fill: 'none', stroke: 'currentColor', strokeWidth: 1.5 },
      h('circle', { cx: '12', cy: '12', r: '10' }),
      h('path', { d: 'M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10A15.3 15.3 0 0 1 12 2z' }),
      h('line', { x1: '2', y1: '12', x2: '22', y2: '12' })
    ),
    order: 8,
    component: XboxDnsPage,
  });
})(window.ZPUI || {});
