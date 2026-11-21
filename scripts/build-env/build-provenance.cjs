const {spawnSync} = require('node:child_process');
const {createHash} = require('node:crypto');
const fs = require('node:fs');
const path = require('node:path');
const {digest, verify} = require('./artifacts.cjs');

const web = fs.realpathSync(process.env.ABA_WEB_ROOT ?? '/apps/web');
const source = fs.realpathSync(process.env.ABA_WEB_SOURCE ?? '/web-source');
const evidence = fs.realpathSync(process.env.ABA_BUILD_EVIDENCE_DIR ?? '/build-evidence');
const contains = (parent, child) => {
  const relative = path.relative(parent, child);
  return relative === '' || relative !== '..' && !relative.startsWith('..' + path.sep);
};
if ([web, source].some(directory => contains(directory, evidence) || contains(evidence, directory))) {
  throw Error('build evidence requires a separate directory');
}

const dir = fs.mkdtempSync(path.join(evidence, 'build.'));
const steps = [
  ['node', path.join(__dirname, 'artifacts.cjs'), 'copy'],
  ['sh', path.join(__dirname, 'check-web.sh')],
  ['npm', 'run', 'build'],
  ['node', path.join(__dirname, 'artifacts.cjs'), 'record'],
  ['node', path.join(__dirname, 'artifacts.cjs'), 'verify'],
];
const git = spawnSync('git', ['-C', source, 'rev-parse', '--verify', 'HEAD'], {encoding: 'utf8'});
const isCommit = value => /^(?:[a-f0-9]{40}|[a-f0-9]{64})$/.test(value);
const head = git.status === 0 ? git.stdout?.trim() : '';
const declaredCommit = process.env.ABA_BUILD_SOURCE_COMMIT ?? '';
const commit = isCommit(head) ? head : isCommit(declaredCommit) ? declaredCommit : null;
const commitSource = isCommit(head) ? 'checkout' : commit ? 'caller' : 'unavailable';
const receipt = {
  schema: 1, startedAt: new Date().toISOString(), commit, commitSource,
  sourceSHA256: null, pid: process.pid, ppid: process.ppid, steps: [],
};
const receiptPath = path.join(dir, 'provenance.json');
const record = () => {
  const pending = path.join(dir, 'provenance.tmp');
  fs.writeFileSync(pending, JSON.stringify(receipt, null, 2) + '\n', {mode: 0o600});
  fs.renameSync(pending, receiptPath);
};
record();
try {
  if (declaredCommit && (!isCommit(declaredCommit) || (isCommit(head) && declaredCommit !== head))) {
    throw Error('invalid or mismatched build commit');
  }
  // An earlier runtime may be stale when rebuilding after a source edit.
  // An E01 refusal is different: preserve that workspace instead of building over it.
  try {
    verify(web, source);
  } catch (error) {
    if (error.message === 'standalone changed: E01 directory present') throw error;
  }
  receipt.sourceSHA256 = digest(source);
  for (const [program, ...args] of steps) {
    if (args.at(-1) === 'record') {
      fs.cpSync(path.join(web, '.next/static'), path.join(web, '.next/standalone/.next/static'), {recursive: true});
      if (fs.existsSync(path.join(web, 'public'))) {
        fs.cpSync(path.join(web, 'public'), path.join(web, '.next/standalone/public'), {recursive: true});
      }
    }
    const step = {argv: [program, ...args], startedAt: new Date().toISOString()};
    receipt.steps.push(step);
    record();
    const child = spawnSync(program, args, {cwd: web, stdio: 'inherit'});
    Object.assign(step, {pid: child.pid, exit: child.status, signal: child.signal, finishedAt: new Date().toISOString()});
    record();
    if (child.status !== 0) throw Error('build step failed: ' + program);
  }
  const proof = verify(web, source);
  if (proof.source !== receipt.sourceSHA256) throw Error('source changed after build admission');
  receipt.standaloneSHA256 = proof.standalone;
  receipt.proofSHA256 = createHash('sha256').update(fs.readFileSync(path.join(web, '.next/aba-build-proof.json'))).digest('hex');
  receipt.status = 'verified';
} catch (error) {
  receipt.status = 'failed';
  console.error(error.message);
  process.exitCode = 42;
} finally {
  receipt.finishedAt = new Date().toISOString();
  record();
}
