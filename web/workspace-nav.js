const workspaceMenu = document.querySelector('.tool-menu');
const compact = matchMedia('(max-width: 820px)');
function fitMenu() { workspaceMenu.open = !compact.matches; }
fitMenu();
compact.addEventListener('change', fitMenu);
workspaceMenu.addEventListener('click', event => {
  if (compact.matches && event.target.closest('a')) workspaceMenu.open = false;
});
document.addEventListener('keydown', event => {
  if (event.key === 'Escape' && compact.matches && workspaceMenu.open) {
    workspaceMenu.open = false;
    workspaceMenu.querySelector('summary').focus();
  }
});
