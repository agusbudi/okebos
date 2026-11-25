CREATE TABLE IF NOT EXISTS opd (
  id INT AUTO_INCREMENT PRIMARY KEY,
  kode VARCHAR(20) NOT NULL UNIQUE,
  nama VARCHAR(120) NOT NULL
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS users (
  id INT AUTO_INCREMENT PRIMARY KEY,
  username VARCHAR(50) NOT NULL UNIQUE,
  nama VARCHAR(100) NOT NULL,
  password_hash VARCHAR(100) NOT NULL,
  role ENUM('admin','opd','walikota') NOT NULL,
  opd_id INT NULL,
  FOREIGN KEY (opd_id) REFERENCES opd(id)
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS pekerjaan (
  id INT AUTO_INCREMENT PRIMARY KEY,
  opd_id INT NOT NULL,
  nama VARCHAR(200) NOT NULL,
  nilai_kontrak BIGINT NOT NULL DEFAULT 0,
  tgl_mulai DATE NOT NULL,
  tgl_selesai DATE NOT NULL,
  status ENUM('berjalan','selesai') NOT NULL DEFAULT 'berjalan',
  FOREIGN KEY (opd_id) REFERENCES opd(id)
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS progress_pekerjaan (
  id INT AUTO_INCREMENT PRIMARY KEY,
  pekerjaan_id INT NOT NULL,
  persen TINYINT UNSIGNED NOT NULL,
  keterangan VARCHAR(255) NOT NULL DEFAULT '',
  tanggal DATE NOT NULL,
  created_by INT NOT NULL,
  FOREIGN KEY (pekerjaan_id) REFERENCES pekerjaan(id),
  FOREIGN KEY (created_by) REFERENCES users(id)
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS bapenda_cashflow (
  id INT AUTO_INCREMENT PRIMARY KEY,
  tanggal DATE NOT NULL,
  kode_rekening VARCHAR(30) NOT NULL,
  uraian VARCHAR(150) NOT NULL,
  jumlah BIGINT NOT NULL,
  batch_id VARCHAR(30) NOT NULL,
  INDEX (batch_id)
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS sync_log (
  id INT AUTO_INCREMENT PRIMARY KEY,
  waktu DATETIME NOT NULL,
  batch_id VARCHAR(30) NOT NULL,
  jumlah_baris INT NOT NULL,
  triggered_by INT NOT NULL,
  FOREIGN KEY (triggered_by) REFERENCES users(id)
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS budget_allocations (
  id INT AUTO_INCREMENT PRIMARY KEY,
  pekerjaan_id INT NOT NULL,
  cashflow_id INT NOT NULL,
  nominal BIGINT NOT NULL,
  dibuat_oleh INT NOT NULL,
  dibuat_pada DATETIME NOT NULL,
  FOREIGN KEY (pekerjaan_id) REFERENCES pekerjaan(id),
  FOREIGN KEY (cashflow_id) REFERENCES bapenda_cashflow(id),
  FOREIGN KEY (dibuat_oleh) REFERENCES users(id)
) ENGINE=InnoDB;
