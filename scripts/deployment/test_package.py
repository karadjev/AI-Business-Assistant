import unittest

import package


class CompilerAdmissionTests(unittest.TestCase):
    def metadata(self, overrides=None, omit=()):
        versions = {part: 'go1.26.11' for part in package.GO_PARTS}
        versions.update(overrides or {})
        return '\n'.join('/input/' + part + ': ' + version + '\n\tpath\tsynthetic' for part, version in versions.items() if part not in omit)

    def test_exact_patched_producer_all_roles(self):
        self.assertEqual(set(package.parse_go_versions(self.metadata())), set(package.GO_PARTS))

    def test_old_or_mixed_toolchains_are_refused(self):
        for part in package.GO_PARTS:
            with self.subTest(part=part):
                with self.assertRaises(ValueError):
                    package.parse_go_versions(self.metadata({part: 'go1.26.2'}))

    def test_missing_or_duplicate_metadata_is_refused(self):
        for part in package.GO_PARTS:
            with self.subTest(part=part):
                with self.assertRaises(ValueError):
                    package.parse_go_versions(self.metadata(omit=(part,)))
        with self.assertRaises(ValueError):
            package.parse_go_versions(self.metadata() + '\n/input/api: go1.26.11')

    def test_unknown_unreviewed_producer_is_refused(self):
        for version in ('devel go1.28', 'go1.27.4', 'go1.26.11 unsafe', ''):
            with self.subTest(version=version):
                with self.assertRaises(ValueError):
                    package.parse_go_versions(self.metadata({'api': version}))


if __name__ == '__main__':
    unittest.main()
