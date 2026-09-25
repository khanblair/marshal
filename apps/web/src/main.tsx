import { render } from "solid-js/web";
// Boots the fake daemon first: it sets `window.M`, applies the theme, and marks `M.S.ready`.
import "~/mock";
import { AppRoot } from "~/app/AppRoot";
import "./styles/app.css";

const root = document.getElementById("root");
if (root) render(() => <AppRoot />, root);
