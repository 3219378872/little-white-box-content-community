-- MANUAL maintenance-window migration, NEVER part of automatic patch replay.
-- Preconditions: patch applied; stop count consumer; stop all old/new Interaction
-- writers; drain in-flight writes; deploy snapshot-producing writers but leave
-- them stopped. Back up both databases. Run on their shared authoritative MySQL.
-- Hold the maintenance window until COMMIT succeeds and counts are verified.
-- REQUIRED: set @legacy_stats_seq_floor in this same connection to the verified
-- maximum stats_seq of live Search documents AND every retained/replayable legacy
-- lifecycle/count event (event_time where stats_seq is absent). Pause Content
-- comment/post writers too. No clock-based estimate is sufficient. For an empty
-- installation with no historical Search/broker/outbox state, explicitly set 1.
-- An unset/invalid floor fails before any count data changes; do not use --force.
USE `xbh_content`;
DROP TEMPORARY TABLE IF EXISTS count_stats_seed_guard;
CREATE TEMPORARY TABLE count_stats_seed_guard (
 floor BIGINT NOT NULL CHECK (floor > 0 AND floor < 9223372036854775807)
);
INSERT INTO count_stats_seed_guard
 SELECT IF(@legacy_stats_seq_floor >= COALESCE(MAX(stats_seq), 0), @legacy_stats_seq_floor, NULL) FROM xbh_content.post;
START TRANSACTION;
INSERT INTO xbh_interaction.action_count
 (target_id, target_type, like_count, favorite_count, comment_count, share_count, revision)
SELECT target_id, target_type, SUM(likes), SUM(favorites), 0, 0, 1 FROM (
 SELECT target_id, target_type, IF(status = 1, 1, 0) AS likes, 0 AS favorites FROM xbh_interaction.like_record
 UNION ALL
 SELECT post_id AS target_id, 1 AS target_type, 0 AS likes, IF(status = 1, 1, 0) AS favorites FROM xbh_interaction.favorite
 UNION ALL
 SELECT target_id, target_type, 0 AS likes, 0 AS favorites FROM xbh_interaction.action_count
) AS authoritative_relations GROUP BY target_id, target_type
ON DUPLICATE KEY UPDATE like_count = VALUES(like_count), favorite_count = VALUES(favorite_count), revision = revision + 1;
UPDATE xbh_content.post p LEFT JOIN xbh_interaction.action_count a ON a.target_id = p.id AND a.target_type = 1
 SET p.like_count = COALESCE(a.like_count, 0), p.favorite_count = COALESCE(a.favorite_count, 0);
UPDATE xbh_content.comment c LEFT JOIN xbh_interaction.action_count a ON a.target_id = c.id AND a.target_type = 2
 SET c.like_count = COALESCE(a.like_count, 0);
UPDATE xbh_content.post SET stats_seq = GREATEST(stats_seq, @legacy_stats_seq_floor, 1);
INSERT INTO xbh_content.content_count_projection (target_type, target_id, revision)
 SELECT IF(target_type = 1, 'post', 'comment'), target_id, revision FROM xbh_interaction.action_count WHERE target_type IN (1, 2)
 ON DUPLICATE KEY UPDATE revision = VALUES(revision);
INSERT INTO xbh_content.content_count_projection_control (id, legacy_reconciled) VALUES (1, TRUE)
 ON DUPLICATE KEY UPDATE legacy_reconciled = TRUE;
COMMIT;
DROP TEMPORARY TABLE count_stats_seed_guard;
-- Invalidate/rebuild affected model caches and search count projections before
-- resuming readers. Resume only new writers and the new count consumer together.
