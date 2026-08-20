INSERT INTO users (id,email,name,timezone) VALUES
('10000000-0000-0000-0000-000000000001','alex@example.com','Alex Morgan','UTC')
ON CONFLICT DO NOTHING;

INSERT INTO notification_preferences (user_id,channel,frequency) VALUES
('10000000-0000-0000-0000-000000000001','email','daily'),
('10000000-0000-0000-0000-000000000001','push','daily'),
('10000000-0000-0000-0000-000000000001','in_app','weekly')
ON CONFLICT DO NOTHING;

INSERT INTO notification_rules (user_id,name,trigger_type,event_type,channel,subject_template,body_template)
VALUES ('10000000-0000-0000-0000-000000000001','Task summary','event','task_completed','in_app','Your summary is ready','Nice work. Your new {{event_type}} summary is ready.')
ON CONFLICT DO NOTHING;
