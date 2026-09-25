import { focusInitial, trapTab } from "./focus-trap";

function box(html: string) {
  const el = document.createElement("div");
  el.innerHTML = html;
  document.body.append(el);
  return el;
}

describe("focus trap", () => {
  afterEach(() => {
    document.body.innerHTML = "";
  });

  it("ignores other keys, empty boxes, and Tab in the middle", () => {
    const el = box('<button id="a">a</button><input id="b"><button id="c">c</button>');
    (el.querySelector("#b") as HTMLElement).focus();
    expect(trapTab(new KeyboardEvent("keydown", { key: "Enter" }), el)).toBe(false);
    expect(trapTab(new KeyboardEvent("keydown", { key: "Tab" }), el)).toBe(false);
    expect(trapTab(new KeyboardEvent("keydown", { key: "Tab" }), box("<p>none</p>"))).toBe(false);
  });

  it("skips disabled buttons", () => {
    const el = box('<button id="a">a</button><button id="c" disabled>c</button>');
    (el.querySelector("#a") as HTMLElement).focus();
    expect(trapTab(new KeyboardEvent("keydown", { key: "Tab" }), el)).toBe(true);
    expect(el.querySelector("#a")).toHaveFocus();
  });

  it("focuses the marked element, or nothing", () => {
    const el = box('<input id="a"><input id="b" data-autofocus>');
    focusInitial(el);
    expect(el.querySelector("#b")).toHaveFocus();
    expect(() => focusInitial(box("<p>none</p>"))).not.toThrow();
  });
});
