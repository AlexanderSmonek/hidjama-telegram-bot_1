-- Supabase SQL Schema for HGN Access Bot

-- Masters table
CREATE TABLE IF NOT EXISTS masters (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    code TEXT NOT NULL,
    contact TEXT,
    gender TEXT NOT NULL CHECK (gender IN ('male', 'female')),
    active BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Packages table
CREATE TABLE IF NOT EXISTS packages (
    key TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT,
    price INTEGER NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Slots table
CREATE TABLE IF NOT EXISTS slots (
    id SERIAL PRIMARY KEY,
    date TEXT NOT NULL,
    time TEXT NOT NULL,
    gender TEXT NOT NULL CHECK (gender IN ('male', 'female', 'any')),
    master_id TEXT,
    master_name TEXT NOT NULL,
    status TEXT DEFAULT 'free' CHECK (status IN ('free', 'booked', 'cancelled', 'completed', 'no_show')),
    user_id TEXT,
    username TEXT,
    client_name TEXT,
    client_phone TEXT,
    booked_at TIMESTAMP WITH TIME ZONE,
    source TEXT CHECK (source IN ('nfc', 'qr', 'link', 'bot')),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    FOREIGN KEY (master_id) REFERENCES masters(id)
);

-- Insert initial masters
INSERT INTO masters (id, name, code, contact, gender, active) VALUES
('adam', 'Адам', '1846', '', 'male', true),
('aslan', 'Аслан', '1144', '', 'male', true),
('deni', 'Дени', '0989', '+79267640131', 'male', true),
('diana', 'Диана', '8567', '+79374084740', 'female', true),
('muhammad', 'Мухаммад', '1231', '+79637149002', 'male', true)
ON CONFLICT (id) DO NOTHING;

-- Insert initial packages
INSERT INTO packages (key, name, description, price) VALUES
('complex', 'Комплексная хиджама', 'Перезапуск общего состояния и регуляции организма.', 3500),
('upper', '+ Верхние конечности', 'Дополнение к комплексной хиджаме.', 4500),
('lower', '+ Нижние конечности', 'Дополнение к комплексной хиджаме.', 5500),
('individual', 'Индивидуальная', 'Персональная процедура.', 6500),
('cosmetology', 'Косметологическая (лицо)', 'Процедура для лица.', 5500)
ON CONFLICT (key) DO NOTHING;

-- Create indexes for better performance
CREATE INDEX IF NOT EXISTS idx_slots_date_time ON slots(date, time);
CREATE INDEX IF NOT EXISTS idx_slots_status ON slots(status);
CREATE INDEX IF NOT EXISTS idx_slots_user_id ON slots(user_id);
CREATE INDEX IF NOT EXISTS idx_masters_active ON masters(active);
