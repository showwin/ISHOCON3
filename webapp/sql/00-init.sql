CREATE DATABASE IF NOT EXISTS ishocon3 DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci;

DROP USER IF EXISTS 'ishocon'@'%';
CREATE USER IF NOT EXISTS 'ishocon'@'%' IDENTIFIED BY 'ishocon';
GRANT ALL ON ishocon3.* TO 'ishocon'@'%';
