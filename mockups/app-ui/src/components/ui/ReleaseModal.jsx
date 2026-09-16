import { useState, useEffect } from 'react';
import Modal from './Modal';
import { api, openExternal } from '../../api';
import { useT } from '../../i18n';
import { ExternalLink, Loader2, Tag } from 'lucide-react';

const ROUTES = {
  zpui: '/api/updates/release-notes',
  zapret: '/api/updates/zapret-release-notes',
};

// Рендер release notes без markdown-зависимостей: ## заголовки, списки
// «*» и вложенные (2 пробела), **жирный**, `код`, URL — кликабельны.
// Строка «Full Changelog: <url>» превращается в ссылку.
function renderNotes(body, onOpenLink) {
  if (!body) return null;
  const lines = body.replace(/\r\n/g, '\n').split('\n');
  const out = [];
  let list = [];

  const flushList = () => {
    if (list.length) {
      out.push(<ul key={'ul' + out.length} className="rn-list">{list}</ul>);
      list = [];
    }
  };

  const inline = (text, keyPrefix) => {
    const parts = [];
    const re = /(\*\*[^*]+\*\*|`[^`]+`|https?:\/\/[^\s)]+)/g;
    let last = 0, m, i = 0;
    while ((m = re.exec(text)) !== null) {
      if (m.index > last) parts.push(text.slice(last, m.index));
      const tok = m[0];
      if (tok.startsWith('**')) parts.push(<b key={keyPrefix + i}>{tok.slice(2, -2)}</b>);
      else if (tok.startsWith('`')) parts.push(<code key={keyPrefix + i} className="rn-code">{tok.slice(1, -1)}</code>);
      else parts.push(
        <a key={keyPrefix + i} className="rn-link" href="#" onClick={(e) => { e.preventDefault(); onOpenLink(tok); }}>{tok}</a>
      );
      last = m.index + tok.length;
      i++;
    }
    if (last < text.length) parts.push(text.slice(last));
    return parts;
  };

  lines.forEach((raw, idx) => {
    const line = raw.replace(/\s+$/, '');
    if (/^#{2,3}\s+/.test(line)) {
      flushList();
      const text = line.replace(/^#+\s+/, '');
      if (/^new contributors/i.test(text)) return; // служебная секция — схлопываем
      out.push(<div key={'h' + idx} className="rn-heading">{text}</div>);
      return;
    }
    if (/^\s{2}\*\s+/.test(line)) {
      list.push(<li key={'li' + idx} className="rn-li rn-li-sub">{inline(line.replace(/^\s+\*\s+/, ''), 's' + idx)}</li>);
      return;
    }
    if (/^\*\s+/.test(line)) {
      list.push(<li key={'li' + idx} className="rn-li">{inline(line.replace(/^\*\s+/, ''), 'l' + idx)}</li>);
      return;
    }
    const full = line.match(/^\s*\*?\s*\**\s*Full Changelog:?\**\s*(\S+)/i);
    if (full) {
      flushList();
      out.push(
        <div key={'fc' + idx} className="rn-full">
          <a className="rn-link" href="#" onClick={(e) => { e.preventDefault(); onOpenLink(full[1]); }}>Full Changelog</a>
        </div>
      );
      return;
    }
    flushList();
    if (line.trim() === '') return;
    out.push(<div key={'p' + idx} className="rn-p">{inline(line, 'p' + idx)}</div>);
  });
  flushList();
  return out;
}

export default function ReleaseModal({ open, onClose, currentVersion, latestVersion, target = 'zpui', version = '' }) {
  const { t } = useT();
  const [info, setInfo] = useState(null);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!open) return;
    setLoading(true);
    setInfo(null);
    const route = ROUTES[target] || ROUTES.zpui;
    const q = version ? '?version=' + encodeURIComponent(version) : '';
    api('GET', route + q).then(d => {
      setInfo(d);
      setLoading(false);
    });
  }, [open, target, version]);

  const formatDate = (dateStr) => {
    if (!dateStr) return '';
    try {
      const d = new Date(dateStr);
      return d.toLocaleDateString();
    } catch {
      return dateStr;
    }
  };

  // Показываем именно запрошенную версию; tag из ответа — только уточнение.
  const tagName = info?.tag_name ? info.tag_name.replace(/^v/, '') : (version || latestVersion);

  return (
    <Modal
      title={t('update.releaseTitle')}
      onClose={onClose}
      open={open}
      wide
      icon={<Tag size={18} strokeWidth={2} />}
      tone="accent"
      footer={!loading && info && !info.error && (
        <>
          {info.html_url && (
            <button className="btn btn-sm btn-ghost" onClick={() => openExternal(info.html_url)}>
              <ExternalLink size={13} strokeWidth={2} />
              GitHub
            </button>
          )}
          <button className="btn btn-sm" onClick={onClose}>{t('common.close')}</button>
        </>
      )}
    >
      <div className="release-modal">
        {loading && (
          <div className="release-loading">
            <Loader2 size={24} className="spin" />
            <span>{t('update.loadingRelease')}</span>
          </div>
        )}

        {!loading && info?.error && (
          <div className="release-error" role="alert">
            <span>{t('update.releaseError')}</span>
            <code>{info.error}</code>
          </div>
        )}

        {!loading && info && !info.error && (
          <>
            <div className="release-header">
              <div className="release-version-info">
                <span className="release-tag">v{tagName}</span>
                {info.name && info.name !== info.tag_name && (
                  <span className="release-name">{info.name}</span>
                )}
              </div>
              {info.published_at && (
                <span className="release-date">{formatDate(info.published_at)}</span>
              )}
            </div>

            {currentVersion && (
              <div className="release-version-compare">
                <span className="release-ver-current mono">v{currentVersion}</span>
                <span className="release-arrow">→</span>
                <span className="release-ver-new mono">v{tagName}</span>
              </div>
            )}

            {info.body && (
              <div className="release-body">
                <div className="release-notes">{renderNotes(info.body, openExternal)}</div>
              </div>
            )}
          </>
        )}
      </div>
    </Modal>
  );
}
