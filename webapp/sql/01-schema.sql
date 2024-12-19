SET CHARACTER_SET_CLIENT = utf8mb4;
SET CHARACTER_SET_CONNECTION = utf8mb4;

CREATE DATABASE IF NOT EXISTS ishocon3 DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci;

DROP TABLE IF EXISTS `users`;
CREATE TABLE `users` (
  `id` VARCHAR(36) NOT NULL COMMENT 'ユーザID',
  `name` varchar(30) NOT NULL COMMENT 'ユーザ名',
  `hashed_password` varchar(255) NOT NULL COMMENT 'パスワード',
  `salt` varchar(30) NOT NULL COMMENT 'ソルト',
  `is_admin` tinyint(1) NOT NULL DEFAULT 0 COMMENT '管理者フラグ',
  `global_payment_token` varchar(36) NOT NULL COMMENT '支払い用のトークン',
  `api_call_at` datetime(6) DEFAULT NULL COMMENT '最終APIコール',
  `created_at` datetime(6) DEFAULT CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;


DROP TABLE IF EXISTS `trains`;
CREATE TABLE `trains` (
  `id` int NOT NULL AUTO_INCREMENT COMMENT '列車筐体ID',
  `name` varchar(30) NOT NULL COMMENT '列車筐体名',
  `model_name` varchar(30) NOT NULL COMMENT 'モデル名',
  `created_at` datetime(6) DEFAULT CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;


DROP TABLE IF EXISTS `train_models`;
CREATE TABLE `train_models` (
  `name` varchar(30) NOT NULL COMMENT 'モデル名',
  `seat_rows` int NOT NULL COMMENT '座席行数',
  `seat_columns` int NOT NULL COMMENT '座席列数(4列:A-D, 5列:A-E)',
  PRIMARY KEY (`name`),
  UNIQUE KEY `name` (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;


DROP TABLE IF EXISTS `stations`;
CREATE TABLE `stations` (
  `id` varchar(1) NOT NULL COMMENT '駅ID',
  `name` varchar(10) DEFAULT NULL COMMENT '駅名',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;


DROP TABLE IF EXISTS `train_schedules`;
CREATE TABLE `train_schedules` (
  `id` varchar(10) NOT NULL COMMENT '列車ID',
  `train_id` varchar(10) NOT NULL COMMENT '列車筐体ID',
  `departure_at_station_a_to_b` varchar(5) NOT NULL COMMENT '駅A -> 駅Bの出発時刻',
  `departure_at_station_b_to_c` varchar(5) NOT NULL COMMENT '駅B -> 駅Cの出発時刻',
  `departure_at_station_c_to_d` varchar(5) NOT NULL COMMENT '駅C -> 駅Dの出発時刻',
  `departure_at_station_d_to_e` varchar(5) NOT NULL COMMENT '駅D -> 駅Eの出発時刻',
  `departure_at_station_e_to_d` varchar(5) NOT NULL COMMENT '駅E -> 駅Dの出発時刻',
  `departure_at_station_d_to_c` varchar(5) NOT NULL COMMENT '駅D -> 駅Cの出発時刻',
  `departure_at_station_c_to_b` varchar(5) NOT NULL COMMENT '駅C -> 駅Bの出発時刻',
  `departure_at_station_b_to_a` varchar(5) NOT NULL COMMENT '駅B -> 駅Aの出発時刻',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;


DROP TABLE IF EXISTS `seat_row_reservations`;
CREATE TABLE `seat_row_reservations` (
  `id` int NOT NULL AUTO_INCREMENT COMMENT '座席行状態ID',
  `train_id` int NOT NULL COMMENT '列車筐体ID',
  `schedule_id` varchar(10)  NOT NULL COMMENT '列車ID',
  `from_station_id` varchar(1)  NOT NULL COMMENT '出発駅ID',
  `to_station_id` varchar(1) NOT NULL COMMENT '到着駅ID',
  `seat_row` int NOT NULL COMMENT '座席行番号',
  `a_is_available` tinyint(1) NOT NULL DEFAULT 1 COMMENT 'A席の空き状況',
  `b_is_available` tinyint(1) NOT NULL DEFAULT 1 COMMENT 'B席の空き状況',
  `c_is_available` tinyint(1) NOT NULL DEFAULT 1 COMMENT 'C席の空き状況',
  `d_is_available` tinyint(1) NOT NULL DEFAULT 1 COMMENT 'D席の空き状況',
  `e_is_available` tinyint(1) NOT NULL DEFAULT 1 COMMENT 'E席の空き状況',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;


DROP TABLE IF EXISTS `reservation_locks`;
CREATE TABLE `reservation_locks` (
  `schedule_id` varchar(10) NOT NULL COMMENT '列車ID',
  `created_at` datetime(6) DEFAULT CURRENT_TIMESTAMP(6) COMMENT 'ロック作成日時',
  PRIMARY KEY (`schedule_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;


DROP TABLE IF EXISTS `reservations`;
CREATE TABLE `reservations` (
  `id` varchar(26) NOT NULL COMMENT '予約ID',
  `user_id` varchar(36) NOT NULL COMMENT 'ユーザID',
  `schedule_id` varchar(10) NOT NULL COMMENT '列車ID',
  `from_station_id` varchar(1) NOT NULL COMMENT '出発駅ID',
  `to_station_id` varchar(1) NOT NULL COMMENT '到着駅ID',
  `departure_at` varchar(5) NOT NULL COMMENT '出発日時',
  `entry_token` varchar(36) NOT NULL COMMENT '改札口入場トークン',
  `created_at` datetime(6) DEFAULT CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;


DROP TABLE IF EXISTS `reservation_seats`;
CREATE TABLE `reservation_seats` (
  `id` int NOT NULL AUTO_INCREMENT COMMENT '予約席ID',
  `reservation_id` varchar(26) NOT NULL COMMENT '予約ID',
  `seat` varchar(4) NOT NULL COMMENT '座席 (例: "1-A", "12-E")',
  `created_at` datetime(6) DEFAULT CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;


DROP TABLE IF EXISTS `reservation_qr_images`;
CREATE TABLE `reservation_qr_images` (
  `id` varchar(36) NOT NULL COMMENT 'QRコードID',
  `reservation_id` varchar(26) NOT NULL COMMENT '予約ID',
  `image` longblob NOT NULL COMMENT 'QRコード画像',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;


DROP TABLE IF EXISTS `payments`;
CREATE TABLE `payments` (
  `id` int NOT NULL AUTO_INCREMENT COMMENT '支払いID',
  `user_id` varchar(36) NOT NULL COMMENT 'ユーザID',
  `reservation_id` varchar(26) NOT NULL COMMENT '予約ID',
  `amount` int NOT NULL COMMENT '金額',
  `is_captured` tinyint(1) NOT NULL DEFAULT 0 COMMENT '支払い済フラグ',
  `is_refunded` tinyint(1) NOT NULL DEFAULT 0 COMMENT '返金済フラグ',
  `created_at` datetime(6) DEFAULT CURRENT_TIMESTAMP(6) COMMENT '支払い作成日時',
  `updated_at` datetime(6) DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6) COMMENT '支払い更新日時',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;


DROP TABLE IF EXISTS `entries`;
CREATE TABLE `entries` (
  `id` int NOT NULL AUTO_INCREMENT COMMENT '入場記録ID',
  `reservation_id` varchar(26) NOT NULL COMMENT '予約ID',
  `entry_at` datetime(6) DEFAULT CURRENT_TIMESTAMP(6) COMMENT '入場日時',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;


DROP TABLE IF EXISTS `settings`;
CREATE TABLE `settings` (
  `initialized_at` datetime(6) DEFAULT CURRENT_TIMESTAMP(6) COMMENT '初期化日時 システム内の時計の基準時として使用'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
