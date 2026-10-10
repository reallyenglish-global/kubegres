#!/usr/bin/env python3
"""Opt-in real PostgreSQL bootstrap tests; no Kubernetes or shared services.
Run: PG_BINDIR=/path/to/postgresql/bin python3 tests/bootstrap_native.py -v
Requires a non-root user, PostgreSQL client/server tools, and openssl.
"""
import os
import pathlib
import shutil
import signal
import socket
import subprocess
import tempfile
import time
import unittest

BIN = pathlib.Path(os.environ.get('PG_BINDIR', '/usr/lib/postgresql/18/bin'))
SCRIPT = pathlib.Path(__file__).resolve().parents[1] / 'internal/controller/spec/template/bootstrap.sh'

class NativeBootstrap(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        if os.getuid() == 0:
            raise RuntimeError('run as a non-root user')
        cls.tmp = tempfile.TemporaryDirectory(prefix='kb-native-')
        cls.root = pathlib.Path(cls.tmp.name)
        cls.env = dict(os.environ, PATH=str(BIN)+':'+os.environ['PATH'])
        # Do not inherit libpq routing or credentials from the host.
        for key in list(cls.env):
            if key.startswith('PG') and key != 'PG_BINDIR':
                del cls.env[key]
        with socket.socket() as sock:
            sock.bind(('127.0.0.1', 0))
            cls.port = str(sock.getsockname()[1])
        cls.source = cls.root/'source'
        cls.call(['initdb', '-D', str(cls.source), '--auth=trust', '--username=postgres'])
        cls.call(['openssl', 'req', '-x509', '-newkey', 'rsa:2048', '-nodes', '-days', '1', '-subj', '/CN=localhost', '-addext', 'subjectAltName=DNS:localhost', '-keyout', str(cls.source/'server.key'), '-out', str(cls.source/'server.crt')])
        (cls.source/'server.key').chmod(0o600)
        with (cls.source/'postgresql.conf').open('a') as f:
            f.write("\nssl=on\nlisten_addresses='127.0.0.1'\nport="+cls.port+"\nunix_socket_directories='"+str(cls.root)+"'\n")
        (cls.source/'pg_hba.conf').write_text('local all all trust\nhostssl all all 127.0.0.1/32 scram-sha-256\n')
        cls.call(['pg_ctl', '-D', str(cls.source), '-l', str(cls.root/'source.log'), '-w', 'start'])
        cls.call(['psql', '-h', str(cls.root), '-p', cls.port, '-U', 'postgres', '-d', 'postgres', '-v', 'ON_ERROR_STOP=1', '-c', "CREATE ROLE importer LOGIN PASSWORD 'synthetic-source'; CREATE ROLE noaccess LOGIN PASSWORD 'synthetic-source';"])
        cls.call(['createdb', '-h', str(cls.root), '-p', cls.port, '-U', 'postgres', '-O', 'importer', 'source_db'])
        cls.call(['psql', '-h', str(cls.root), '-p', cls.port, '-U', 'importer', '-d', 'source_db', '-v', 'ON_ERROR_STOP=1', '-c', "CREATE TABLE samples(id integer PRIMARY KEY, value text); INSERT INTO samples VALUES (1,'unicode 日本語'),(2,'quote '' and newline'),(3,NULL); CREATE SEQUENCE sample_seq START 41; SELECT nextval('sample_seq');"])

    @classmethod
    def call(cls, args, env=None, check=True):
        result = subprocess.run(args, env=env or cls.env, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
        if check and result.returncode:
            raise AssertionError(str(args[0])+': '+result.stdout)
        return result

    @classmethod
    def tearDownClass(cls):
        cls.call(['pg_ctl', '-D', str(cls.source), '-m', 'immediate', '-w', 'stop'], check=False)
        cls.tmp.cleanup()

    def setUp(self):
        self.volume = self.root/('v'+str(list(type(self).__dict__).index(self._testMethodName)))
        self.volume.mkdir()
        self.target = self.volume/'pgdata'
        self.stage = self.volume/'.kubegres-bootstrap-staging'
        self.e = dict(self.env, PGDATA=str(self.target), POSTGRES_USER='postgres', POSTGRES_PASSWORD='synthetic-destination', POSTGRES_REPLICATION_PASSWORD='synthetic-replication', BOOTSTRAP_SOURCE_PASSWORD='synthetic-source', BOOTSTRAP_SOURCE_HOST='localhost', BOOTSTRAP_SOURCE_PORT=self.port, BOOTSTRAP_SOURCE_DATABASE='source_db', BOOTSTRAP_SOURCE_USERNAME='importer', BOOTSTRAP_DESTINATION_DATABASE='app_db', BOOTSTRAP_CA_FILE=str(self.source/'server.crt'), BOOTSTRAP_TIMEOUT_SECONDS='30', BOOTSTRAP_CR_UID='synthetic-uid', BOOTSTRAP_PVC_NAME='synthetic-pvc')

    def tearDown(self):
        for data in (self.target, self.stage/'data'):
            if data.exists():
                self.call(['pg_ctl', '-D', str(data), '-m', 'immediate', '-w', 'stop'], check=False)

    def bootstrap(self, success=True, error=None):
        r = self.call(['/bin/sh', str(SCRIPT)], self.e, check=False)
        print(self._testMethodName, 'bootstrap exit', r.returncode, r.stdout.strip(), flush=True)
        self.assertEqual(r.returncode == 0, success, r.stdout)
        if error:
            self.assertIn(error, r.stdout)
        for secret in ('synthetic-source', 'synthetic-destination', 'synthetic-replication'):
            self.assertNotIn(secret, r.stdout)
        if not success:
            self.assertFalse(self.target.exists(), 'failed import published PGDATA')
        return r

    def test_import_and_completed_identity(self):
        self.bootstrap()

        # Use a separate ephemeral TCP port to prove destination password authentication.
        with socket.socket() as s:
            s.bind(('127.0.0.1', 0)); port = str(s.getsockname()[1])
        self.call(['pg_ctl', '-D', str(self.target), '-l', str(self.volume/'dest.log'), '-o', '-c listen_addresses=127.0.0.1 -c port='+port+' -c unix_socket_directories='+str(self.volume), '-w', 'start'])
        args = ['psql', '-h', '127.0.0.1', '-p', port, '-U', 'postgres', '-d', 'app_db', '-At', '-c']
        env = dict(self.env, PGPASSWORD='synthetic-destination')
        self.assertEqual(self.call(args+["SELECT id || ':' || coalesce(value,'NULL') FROM samples ORDER BY id"], env).stdout.strip(), "1:unicode 日本語\n2:quote ' and newline\n3:NULL")
        self.assertEqual(self.call(args+["SELECT nextval('sample_seq')"], env).stdout.strip(), '42')
        self.assertEqual(self.call(args+["SELECT rolreplication FROM pg_roles WHERE rolname='replication'"], env).stdout.strip(), 't')
        self.assertNotEqual(self.call(args+['SELECT 1'], dict(env, PGPASSWORD='synthetic-source'), check=False).returncode, 0)
        self.call(['pg_ctl', '-D', str(self.target), '-m', 'fast', '-w', 'stop'])
        self.e['BOOTSTRAP_SOURCE_PORT']='1'
        self.bootstrap()  # marker prevents reimport even with unreachable source
        for key in ('BOOTSTRAP_CR_UID', 'BOOTSTRAP_PVC_NAME'):
            original=self.e[key]; self.e[key]='different'
            r=self.call(['/bin/sh',str(SCRIPT)], self.e, check=False)
            self.assertNotEqual(r.returncode,0); self.assertIn('mismatch',r.stdout)
            self.e[key]=original
        shutil.rmtree(self.target)
        self.e['BOOTSTRAP_COMPLETED']='true'
        self.bootstrap(False, 'completed bootstrap has no valid marker')

    def test_tls_hostname_failure(self):
        self.e['BOOTSTRAP_SOURCE_HOST']='127.0.0.1'
        self.bootstrap(False, 'source export failed')
        self.assertFalse(self.stage.exists())

    def test_tls_ca_failure(self):
        self.e['BOOTSTRAP_CA_FILE']=str(self.volume/'missing-ca')
        self.bootstrap(False, 'source export failed')

    def test_real_dump_permission_failure(self):
        self.e['BOOTSTRAP_SOURCE_USERNAME']='noaccess'
        self.bootstrap(False, 'source export failed')

    def wrapper(self, name, body):
        directory=self.volume/'bin'; directory.mkdir(exist_ok=True)
        path=directory/name
        path.write_text('#!/bin/sh\nset -eu\n'+body+'\n'); path.chmod(0o755)
        self.e['PATH']=str(directory)+':'+self.env['PATH']

    def test_real_restore_corrupt_archive(self):
        # Real pg_dump succeeds; corrupt its artifact, then invoke real pg_restore.
        self.wrapper('pg_restore', 'for archive do :; done\nprintf broken > "$archive"\nexec '+str(BIN/'pg_restore')+' "$@"')
        self.bootstrap(False, 'destination restore failed')
        self.assertFalse(self.stage.exists())

    def interrupt(self, command):
        gate=self.volume/'gate'
        real = str(BIN/command) if command!='mv' else shutil.which('mv')
        self.assertIsNotNone(real)
        self.wrapper(command, 'touch '+str(gate)+'\nwhile :; do sleep 1; done\nexec '+real+' "$@"')
        proc=subprocess.Popen(['/bin/sh',str(SCRIPT)],env=self.e,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,start_new_session=True)
        try:
            deadline=time.monotonic()+40
            while not gate.exists() and proc.poll() is None and time.monotonic()<deadline:
                time.sleep(.05)
            self.assertTrue(gate.exists(), 'bootstrap failed before interruption: '+str(proc.poll()))
        finally:
            if proc.poll() is None: os.killpg(proc.pid,signal.SIGKILL)
            output=proc.communicate(timeout=5)[0]
            print(self._testMethodName,'interrupted exit',proc.returncode,output.decode(),flush=True)
        self.assertFalse(self.target.exists())
        self.e['PATH']=self.env['PATH']
        self.bootstrap()

    def test_interrupted_running_stage_restart(self):
        # Model a postmaster surviving the init shell (as after container/process
        # interruption): run the real pg_ctl in a separate process session.
        self.wrapper('pg_ctl', 'exec setsid '+str(BIN/'pg_ctl')+' "$@"')
        self.interrupt('pg_restore')

    def test_interrupted_publication_restart(self):
        self.interrupt('mv')

    def test_stale_postmaster_pid_restart(self):
        self.stage.mkdir()
        (self.stage/'identity').write_text('cr_uid=synthetic-uid\npvc=synthetic-pvc\n')
        data=self.stage/'data'; data.mkdir()
        # A stopped postmaster can leave its PID file after an abrupt crash.
        (data/'postmaster.pid').write_text('2147483647\n')
        self.bootstrap()

    def test_nonempty_unmarked_target_rejected(self):
        self.target.mkdir(); (self.target/'PG_VERSION').write_text('18\n')
        r=self.call(['/bin/sh',str(SCRIPT)], self.e, check=False)
        self.assertNotEqual(r.returncode,0)
        self.assertIn('non-empty',r.stdout)
        self.assertEqual((self.target/'PG_VERSION').read_text(),'18\n')

    def test_marker_without_database_rejected(self):
        self.target.mkdir()
        (self.target/'.kubegres-bootstrap-complete').write_text('version=kubegres-logical-bootstrap-v1\ncr_uid=synthetic-uid\npvc=synthetic-pvc\n')
        r=self.call(['/bin/sh',str(SCRIPT)], self.e, check=False)
        self.assertNotEqual(r.returncode,0)
        self.assertIn('no PG_VERSION',r.stdout)

    def test_foreign_staging_rejected(self):
        self.stage.mkdir(); (self.stage/'identity').write_text('cr_uid=foreign\npvc=synthetic-pvc\n')
        self.bootstrap(False,'staging identity mismatch')
        self.assertTrue(self.stage.exists())

if __name__ == '__main__':
    unittest.main()
