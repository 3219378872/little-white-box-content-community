-- MANUAL, every Search rebuild; never automatic patch replay.
-- Pause all Content/Interaction/count-sync writers and Search consumers; drain
-- in-flight requests. In this same connection set @legacy_stats_seq_floor to the
-- verified max of SQL sequences, Search documents and every retained/replayable
-- legacy event watermark. Unknown replay bounds block this maintenance action.
-- Do not use --force. Keep writers paused until rebuild+promotion with this same
-- floor completes. This does not recount relations or migrate deletion tombstones.
USE `xbh_content`;
DROP TEMPORARY TABLE IF EXISTS search_stats_seed_guard;
CREATE TEMPORARY TABLE search_stats_seed_guard (
 floor BIGINT NOT NULL CHECK (floor > 0 AND floor < 9223372036854775807)
);
INSERT INTO search_stats_seed_guard
 SELECT IF(@legacy_stats_seq_floor >= COALESCE(MAX(stats_seq), 0), @legacy_stats_seq_floor, NULL) FROM post;
START TRANSACTION;
UPDATE post SET stats_seq = GREATEST(stats_seq, @legacy_stats_seq_floor);
COMMIT;
DROP TEMPORARY TABLE search_stats_seed_guard;
