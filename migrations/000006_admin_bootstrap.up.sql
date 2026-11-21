CREATE TABLE admin_bootstrap (
    singleton BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (singleton)
);

INSERT INTO admin_bootstrap (singleton)
SELECT TRUE WHERE EXISTS (SELECT 1 FROM operators);
