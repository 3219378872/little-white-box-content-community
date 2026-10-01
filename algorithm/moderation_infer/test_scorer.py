import unittest

from algorithm.moderation_infer.scorer import FIXTURE_VERSION, STUB_VERSION, score


class ScorerTest(unittest.TestCase):
    def test_stub_returns_gray_zone_for_every_issue(self):
        version, scores = score("anything", ["A.B", "C.D"], fixture_enabled=False)
        self.assertEqual(STUB_VERSION, version)
        self.assertIn("stub", version)
        self.assertEqual([("A.B", 0.5, 0.5), ("C.D", 0.5, 0.5)], [(s.issue, s.p_yes, s.p_no) for s in scores])

    def test_fixture_markers_only_apply_when_enabled(self):
        text = "x [[fixture:CONTENT.SELF_HARM=0.97]]"
        _, scores = score(text, ["CONTENT.SELF_HARM"], fixture_enabled=False)
        self.assertEqual(0.5, scores[0].p_yes)
        version, scores = score(text, ["CONTENT.SELF_HARM", "MISLEADING.CLAIM"], fixture_enabled=True)
        self.assertEqual(FIXTURE_VERSION, version)
        self.assertAlmostEqual(0.97, scores[0].p_yes)
        self.assertAlmostEqual(0.03, scores[0].p_no)
        self.assertEqual(0.5, scores[1].p_yes)

    def test_duplicate_issues_are_rejected(self):
        with self.assertRaises(ValueError):
            score("", ["A.B", "A.B"], fixture_enabled=False)


if __name__ == "__main__":
    unittest.main()
