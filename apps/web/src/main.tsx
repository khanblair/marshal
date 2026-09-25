import { render } from "solid-js/web";
// Boots the store first: it sets `window.M`, applies the theme, and connects to the daemon. `M.S.ready`
// turns true once the daemon's first data is in (or a connection screen has to draw).
import "~/mock";
import { AppRoot } from "~/app/AppRoot";
import "./styles/app.css";

const root = document.getElementById("root");
if (root) render(() => <AppRoot />, root);
