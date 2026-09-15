-- Создаёт 50 тестовых сессий с привязанными client/executor для нагрузочного теста.
-- Использовать ТОЛЬКО на тестовой БД, не на проде.

DO $$
DECLARE
    i INT;
    client_id BIGINT;
    executor_id BIGINT;
    session_id BIGINT;
BEGIN
    FOR i IN 1..50 LOOP
        INSERT INTO users (telegram_id, language_code)
        VALUES (900000000 + i * 2, 'ru')
        RETURNING id INTO client_id;

        INSERT INTO users (telegram_id, language_code)
        VALUES (900000000 + i * 2 + 1, 'ru')
        RETURNING id INTO executor_id;

        INSERT INTO sessions (title, status, language, client_user_id, executor_user_id)
        VALUES ('Load Test Session ' || i, 'active', 'ru', client_id, executor_id)
        RETURNING id INTO session_id;

        UPDATE users SET active_session_id = session_id WHERE id IN (client_id, executor_id);
    END LOOP;
END $$;