async function fetchLocation() {
  const badges = document.querySelectorAll('#region-badge, #settings-location');
  try {
    const r = await fetch('/location');
    const d = await r.json();
    const loc = d.city && d.region ? `${d.city}, ${d.region_code}` : d.country_name || 'Unknown';
    badges.forEach(el => {
      if (el.id === 'settings-location') {
        el.textContent = `${loc} \u00b7 ${d.org || ''}`;
      } else {
        el.textContent = loc;
      }
    });
  } catch (e) {
    badges.forEach(el => {
      if (el.id === 'settings-location') el.textContent = 'Could not detect';
      else el.textContent = 'Unknown';
    });
  }
}
fetchLocation();
