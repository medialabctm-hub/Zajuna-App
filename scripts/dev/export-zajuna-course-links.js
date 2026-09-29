/**
 * Diagnóstico manual para Zajuna.
 *
 * Uso: abre un curso autenticado, presiona F12 → Console, pega este archivo
 * y guarda el objeto copiado como fixture si necesitas revisar títulos/rutas.
 * No lee ni copia cookies, contraseñas, tokens ni contenido privado adicional.
 */
(function exportZajunaCourseLinks() {
  const origin = 'https://zajuna.sena.edu.co';
  const abs = (href) => {
    const raw = String(href || '').trim();
    if (!raw || raw.startsWith('#') || /^javascript:/i.test(raw)) return '';
    if (raw.startsWith('http')) return raw;
    if (raw.startsWith('/')) return `${origin}${raw}`;
    return `${origin}/zajuna/${raw.replace(/^\//, '')}`;
  };
  const clean = (value) => String(value || '').replace(/\s+/g, ' ').trim();
  const pickLabel = (anchor) => {
    const text = clean(anchor.textContent);
    if (text.length >= 2) return text.slice(0, 160);
    const title = clean(anchor.getAttribute('title'));
    if (title.length >= 2) return title.slice(0, 160);
    const activity = anchor.closest('.activity-item, .activity, li.activity');
    return activity ? clean(activity.textContent).slice(0, 160) : (title || 'actividad');
  };
  const seen = new Set();
  const pageLinks = [];
  document.querySelectorAll(
    '#region-main a[href], .course-content a[href], #page-content a[href], ' +
      '.activities a[href], a.activity[href], li.activity a[href]'
  ).forEach((anchor) => {
    const href = anchor.getAttribute('href') || '';
    if (!/mod\/(forum|page|assign|url|resource)\//i.test(href)) return;
    const path = abs(href);
    if (!path || seen.has(path)) return;
    seen.add(path);
    pageLinks.push({ path, label: pickLabel(anchor) });
  });
  const jump = [...document.querySelectorAll('#jump-to-activity option, select[name="jump"] option')]
    .map((option) => ({ path: abs(option.value), label: clean(option.textContent) }))
    .filter((entry) => entry.path);
  const summary = {
    courseId: new URLSearchParams(location.search).get('id'),
    pageLinkCount: pageLinks.length,
    jumpCount: jump.length,
    pageLinks,
    jump,
  };
  console.log('Zajuna Sync · enlaces y títulos del curso', summary);
  if (typeof copy === 'function') copy(summary);
  return summary;
})();
