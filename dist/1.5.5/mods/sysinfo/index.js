(function(ZPUI) {
  var h = ZPUI.h;
  var useState = ZPUI.useState;
  var useEffect = ZPUI.useEffect;
  var api = ZPUI.api;

  function SysInfoPage(ctx) {
    var info = useState(null)[0];
    var setInfo = useState(null)[1];
    var loading = useState(true)[0];
    var setLoading = useState(true)[1];

    useEffect(function() {
      var alive = true;
      api('GET', '/api/system-resources').then(function(d) {
        if (alive && d) { setInfo(d); setLoading(false); }
      });
      var iv = setInterval(function() {
        api('GET', '/api/system-resources').then(function(d) {
          if (alive && d) setInfo(d);
        });
      }, 3000);
      return function() { alive = false; clearInterval(iv); };
    }, []);

    if (loading) return h('div', {className:'section'}, h('div', {className:'section-title'}, 'Загрузка...'));

    var mem = info.memory || {};
    var cpu = info.cpu || {};

    return h('div', null,
      h('div', {className:'page-title'}, 'Информация о системе'),
      h('div', {className:'dash-grid-3'},
        h('div', {className:'section'},
          h('div', {className:'section-title'}, 'Память'),
          h('div', {className:'stat-card'},
            h('div', {className:'stat-label'}, 'ZPUI'),
            h('div', {className:'stat-value mono'}, (mem.zpui_mb || 0).toFixed(1) + ' MB')
          ),
          h('div', {className:'stat-card'},
            h('div', {className:'stat-label'}, 'Запрет'),
            h('div', {className:'stat-value mono'}, (mem.zapret_mb || 0).toFixed(1) + ' MB')
          ),
          h('div', {className:'stat-card'},
            h('div', {className:'stat-label'}, 'Всего'),
            h('div', {className:'stat-value mono'}, (mem.total_mb || 0).toFixed(1) + ' MB')
          )
        ),
        h('div', {className:'section'},
          h('div', {className:'section-title'}, 'Сеть'),
          h('div', {className:'stat-card'},
            h('div', {className:'stat-label'}, 'IP адрес'),
            h('div', {className:'stat-value mono selectable'}, info.public_ip || '—')
          ),
          h('div', {className:'stat-card'},
            h('div', {className:'stat-label'}, 'Провайдер'),
            h('div', {className:'stat-value'}, info.isp || '—')
          )
        ),
        h('div', {className:'section'},
          h('div', {className:'section-title'}, 'Статус'),
          h('div', {className:'stat-card'},
            h('div', {className:'stat-label'}, 'Запрет'),
            h('div', {className:'stat-value'}, ctx.status && ctx.status.zapret && ctx.status.zapret.running ? 'Запущен' : 'Остановлен')
          ),
          h('div', {className:'stat-card'},
            h('div', {className:'stat-label'}, 'Прокси'),
            h('div', {className:'stat-value'}, ctx.status && ctx.status.proxy && ctx.status.proxy.running ? 'Запущен' : 'Остановлен')
          )
        )
      )
    );
  }

  ZPUI.registerPage('sysinfo', {
    label: 'Система',
    icon: h('svg', {viewBox:'0 0 24 24', width:'24', height:'24', fill:'none', stroke:'currentColor', strokeWidth:1.5},
      h('rect', {x:'2', y:'3', width:'20', height:'14', rx:'2'}),
      h('line', {x1:'8', y1:'21', x2:'16', y2:'21'}),
      h('line', {x1:'12', y1:'17', x2:'12', y2:'21'})
    ),
    order: 30,
    component: SysInfoPage
  });
})(window.ZPUI || {});
