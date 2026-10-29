const files = (await Bun.$`git ls-files --cached --others --exclude-standard '*.go' 'scripts/*'`.text()).trim().split("\n");
const limits = { go: 3600, tooling: 80 };
const counts = { go: 0, tooling: 0 };
for (const file of files.filter((file) => !file.endsWith("_test.go"))) {
  const kind = file.endsWith(".go") ? "go" : "tooling";
  counts[kind] += (await Bun.file(file).text()).split("\n").filter((line) => line.trim()).length;
}
for (const kind of ["go", "tooling"] as const) {
  console.log(`${kind}: ${counts[kind]}/${limits[kind]} nonblank lines`);
  if (counts[kind] > limits[kind]) throw new Error(`${kind} exceeds its LOC budget`);
}
