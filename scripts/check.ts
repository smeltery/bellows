import { JSDOM } from "jsdom";

const dom = new JSDOM("");
Object.assign(globalThis, { window: dom.window, document: dom.window.document });
const { default: mermaid } = await import("mermaid");
mermaid.initialize({ startOnLoad: false });
const files = (await Bun.$`git ls-files --cached --others --exclude-standard '*.md'`.text()).trim().split("\n");
let diagrams = 0;
for (const file of files) {
  const source = await Bun.file(file).text();
  for (const match of source.matchAll(/^```mermaid\s*\n([\s\S]*?)^```/gm)) {
    try {
      await mermaid.parse(match[1]);
      diagrams++;
    } catch (error) {
      throw new Error(`Invalid Mermaid in ${file}`, { cause: error });
    }
  }
}
console.log(`Validated ${diagrams} Mermaid diagrams in ${files.length} Markdown files`);
