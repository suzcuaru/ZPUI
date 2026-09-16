import { useState, useCallback, useMemo } from 'react';
import { api } from '../api';
import { useT } from '../i18n';
import ReportReadyModal from '../components/feedback/ReportReadyModal';
import { FileBarChart, Download, Printer, RefreshCw, AlertCircle } from 'lucide-react';

const PRINT_PALETTE = {
	text: '#1a1a22',
	inset: '#f0f1f4',
	border: '#e2e4ea',
};

function renderMarkdown(md) {
	const lines = md.split('\n');
	const html = [];
	let inTable = false;
	let tableRows = [];
	let inList = false;
	let listItems = [];
	let inCode = false;

	const flushTable = () => {
		if (tableRows.length === 0) return;
		const header = tableRows[0];
		html.push('<table class="rpt-table"><thead><tr>' +
			header.map(c => `<th>${escapeHtml(c)}</th>`).join('') +
			'</tr></thead><tbody>');
		for (let i = 2; i < tableRows.length; i++) {
			html.push('<tr>' + tableRows[i].map(c => `<td>${escapeHtml(c)}</td>`).join('') + '</tr>');
		}
		html.push('</tbody></table>');
		tableRows = [];
	};

	const flushList = () => {
		if (listItems.length === 0) return;
		html.push('<ul class="rpt-list">' + listItems.map(it => `<li>${inlineMd(it)}</li>`).join('') + '</ul>');
		listItems = [];
	};

	for (const raw of lines) {
		const line = raw.trimEnd();

		if (inCode) {
			if (line.startsWith('```')) {
				html.push('</pre>');
				inCode = false;
			} else {
				html.push(escapeHtml(raw));
			}
			continue;
		}

		if (line.startsWith('|') && line.endsWith('|')) {
			if (inList) { flushList(); inList = false; }
			const cells = line.split('|').slice(1, -1).map(c => c.trim());
			if (cells.every(c => /^[-: ]+$/.test(c))) continue;
			inTable = true;
			tableRows.push(cells);
			continue;
		}
		if (inTable) { flushTable(); inTable = false; }

		if (line.startsWith('- ')) {
			inList = true;
			listItems.push(line.slice(2));
			continue;
		}
		if (inList) { flushList(); inList = false; }

		if (line.startsWith('### ')) {
			html.push(`<h3>${inlineMd(line.slice(4))}</h3>`);
		} else if (line.startsWith('## ')) {
			html.push(`<h2>${inlineMd(line.slice(3))}</h2>`);
		} else if (line.startsWith('# ')) {
			html.push(`<h1>${inlineMd(line.slice(2))}</h1>`);
		} else if (line.startsWith('```')) {
			html.push('<pre class="rpt-code">');
			inCode = true;
		} else if (line === '') {
			html.push('');
		} else if (/^[*-]{3,}$/.test(line) || /^={3,}$/.test(line)) {
			html.push('<hr class="rpt-hr" />');
		} else {
			html.push(`<p>${inlineMd(line)}</p>`);
		}
	}
	if (inCode) html.push('</pre>');
	if (inTable) flushTable();
	if (inList) flushList();
	return html.join('\n');
}

function escapeHtml(s) {
	return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}

function inlineMd(s) {
	s = escapeHtml(s);
	s = s.replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>');
	s = s.replace(/`(.+?)`/g, '<code>$1</code>');
	return s;
}

export default function ReportPage({ showToast }) {
  const { t } = useT();
  const [content, setContent] = useState(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const [readyPath, setReadyPath] = useState(null);

  const generate = useCallback(async () => {
    setLoading(true);
    setError(null);
    const r = await api('GET', '/api/report/get');
    setLoading(false);
    if (r && r.content) {
      setContent(r.content);
    } else {
      const msg = (r && r.error) ? r.error : t('report.error');
      setError(msg);
    }
  }, [t]);

	// Сохранение — отдельное действие пользователя: результат открывает
	// модалку с путём и отправкой в Telegram.
	const saveMd = async () => {
		if (!content) return;
		const r = await api('POST', '/api/report/save', { content });
		if (r?.error) {
			showToast(r.error, 'error');
		} else if (r?.path) {
			setReadyPath(r.path);
		}
	};

	const printReport = () => {
		const printWin = window.open('', '_blank');
		if (!printWin) {
			showToast(t('report.printBlocked'), 'error');
			return;
		}
		const live = getComputedStyle(document.documentElement);
		const accent = (live.getPropertyValue('--accent') || '#2b67d4').trim();
		const font = (live.getPropertyValue('--font') || "'Segoe UI',system-ui,sans-serif").trim();
		const mono = (live.getPropertyValue('--font-mono') || "'Consolas',monospace").trim();
		const p = PRINT_PALETTE;
		printWin.document.write(`<!DOCTYPE html><html><head><meta charset="utf-8"><title>ZPUI Report</title>
			<style>
				body { font-family: ${font}; font-size: 13px; color: ${p.text}; max-width: 900px; margin: 0 auto; padding: 24px; }
				h1 { font-size: 22px; border-bottom: 2px solid ${accent}; padding-bottom: 6px; }
				h2 { font-size: 16px; margin-top: 20px; color: ${accent}; }
				h3 { font-size: 14px; margin-top: 14px; }
				table { border-collapse: collapse; width: 100%; margin: 8px 0; font-size: 11.5px; }
				th { background: ${p.inset}; text-align: left; padding: 5px 8px; border: 1px solid ${p.border}; font-weight: 600; }
				td { padding: 4px 8px; border: 1px solid ${p.border}; }
				ul { padding-left: 18px; }
				li { margin: 2px 0; }
				code { background: ${p.inset}; padding: 1px 4px; border-radius: 3px; font-family: ${mono}; }
				hr { border: none; border-top: 1px solid ${p.border}; margin: 16px 0; }
				p { margin: 4px 0; line-height: 1.5; }
				@media print { body { padding: 0; } }
			</style></head><body>`);
		printWin.document.write(renderMarkdown(content));
		printWin.document.write('</body></html>');
		printWin.document.close();
		printWin.focus();
		setTimeout(() => printWin.print(), 300);
	};

	const html = useMemo(() => content ? renderMarkdown(content) : '', [content]);

	return (
		<div className="rpt-page stagger">
			<div className="rpt-toolbar">
				<div className="rpt-toolbar-left">
					<FileBarChart size={16} strokeWidth={2} />
					<span className="rpt-toolbar-title">{t('report.title')}</span>
				</div>
				<div className="rpt-toolbar-actions">
					<button className="btn btn-sm" onClick={generate} disabled={loading}>
						{loading ? <RefreshCw size={13} className="spinning" /> : <RefreshCw size={13} strokeWidth={2} />}
						{t('report.generate')}
					</button>
					{content && (
						<>
							<button className="btn btn-sm" onClick={saveMd}>
								<Download size={13} strokeWidth={2} />
								{t('report.saveMd')}
							</button>
							<button className="btn btn-sm" onClick={printReport}>
								<Printer size={13} strokeWidth={2} />
								{t('report.savePdf')}
							</button>
						</>
					)}
				</div>
			</div>

			<div className="rpt-body" aria-live="polite">
				{loading && (
					<div className="rpt-loading">
						<RefreshCw size={32} className="spinning" />
						<span>{t('report.loading')}</span>
					</div>
				)}
				{error && !loading && (
					<div className="rpt-error" role="alert">
						<AlertCircle size={28} strokeWidth={1.8} />
						<span className="rpt-error-text">{error}</span>
						<button className="btn btn-accent btn-sm" onClick={generate}>
							<RefreshCw size={13} strokeWidth={2} />
							{t('report.retry')}
						</button>
					</div>
				)}
				{!loading && !content && !error && (
					<div className="rpt-empty">
						<FileBarChart size={48} strokeWidth={1.5} />
						<span>{t('report.empty')}</span>
						<button className="btn btn-accent btn-sm" onClick={generate}>{t('report.generate')}</button>
					</div>
				)}
				{content && !loading && !error && (
					<div
						className="rpt-content"
						dangerouslySetInnerHTML={{ __html: html }}
					/>
				)}
			</div>

			<ReportReadyModal
				open={!!readyPath}
				path={readyPath}
				onClose={() => setReadyPath(null)}
				showToast={showToast}
			/>
		</div>
	);
}
