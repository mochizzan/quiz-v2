-- 0007 — seed the school's major list.
-- Spec: docs/superpowers/specs/2026-10-23-schema-and-migrations-design.md §9.5.

INSERT INTO majors (name, code) VALUES ('TKJ','TKJ'), ('RPL','RPL'), ('OTKP','OTKP') ON DUPLICATE KEY UPDATE name = VALUES(name);
