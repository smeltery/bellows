import { fileURLToPath } from "node:url";

const brandDirectory = fileURLToPath(new URL("../assets/brand/", import.meta.url));
const manifest = Bun.file(`${brandDirectory}checksums.json`);
const files = Array.from(new Bun.Glob("**/*").scanSync({ cwd: brandDirectory, onlyFiles: true }))
  .filter((file) => file !== "checksums.json" && !file.endsWith(".md"))
  .sort();
if (files.length === 0) throw new Error("No brand assets found");
const checksums: Record<string, string> = {};
for (const file of files) {
  const bytes = await Bun.file(`${brandDirectory}${file}`).arrayBuffer();
  checksums[file] = new Bun.CryptoHasher("sha256").update(bytes).digest("hex");
}
const expected = `${JSON.stringify(checksums, null, 2)}\n`;
if (Bun.argv.includes("--write")) {
  await Bun.write(manifest, expected);
  console.log(`Recorded SHA-256 checksums for ${files.length} brand files`);
} else {
  if (!(await manifest.exists()) || (await manifest.text()) !== expected) {
    throw new Error("Brand assets changed. Review them, then run bun run brand:update and commit checksums.json.");
  }
  console.log(`Verified SHA-256 checksums for ${files.length} brand files`);
}
