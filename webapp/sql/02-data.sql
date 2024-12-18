insert into stations values ("A", "Arena"), ("B", "Bridge"), ("C", "Cave"), ("D", "Dock"), ("E", "Edge");

-- bcrypt.hashpw("admin".encode(), "$2b$12$Lh9gc29tkPwGw0TPwTqjoO".encode()).decode()
-- bcrypt.hashpw("ishocon".encode(), "$2b$12$W2VwGdhCXQt4ef6Zbrxnke".encode()).decode()
insert into users (`id`, `name`, `hashed_password`, salt, is_admin, global_payment_token) values
("72b4f686-d283-4b63-b8bb-1e1fa7529c11", "admin", "$2b$12$Lh9gc29tkPwGw0TPwTqjoORtwJMvoJAXaXmurqNAcveQnSbHXkf8K", "$2b$12$Lh9gc29tkPwGw0TPwTqjoO", 1, "96d230c6-add1-498e-ac58-95212c9e869e"),
("aea4e2dc-3fb2-42eb-8070-97cf0c129a41", "ishocon", "$2b$12$W2VwGdhCXQt4ef6Zbrxnke4VO6PRVD0uZU5GAmVNjMp88CIT6hbI.", "$2b$12$W2VwGdhCXQt4ef6Zbrxnke", 0, "362f462c-1cb8-4054-890a-944c2b2437ef");

insert into train_models (`name`, `seat_rows`, `seat_columns`) values
("Economy-5", 10, 5),
("Economy-4", 10, 4),
("Business-4", 7, 4),
("First-3", 5, 3),
("Luxury-2", 2, 2);

insert into trains (`id`, `name`, `model_name`) values
(1, "E5001", "Economy-5"),
(2, "E5002", "Economy-5"),
(3, "E5003", "Economy-5"),
(4, "E4001", "Economy-4"),
(5, "E4002", "Economy-4"),
(6, "E4003", "Economy-4"),
(7, "B4001", "Business-4"),
(8, "B4002", "Business-4"),
(9, "F3001", "First-3"),
(10, "L2001", "Luxury-2");

insert into train_schedules (`id`, `train_id`, `departure_at_station_a_to_b`, `departure_at_station_b_to_c`, `departure_at_station_c_to_d`, `departure_at_station_d_to_e`, `departure_at_station_e_to_d`, `departure_at_station_d_to_c`, `departure_at_station_c_to_b`, `departure_at_station_b_to_a`) values
("E5001-1", 1, "00:10", "00:20", "00:30", "00:40", "00:50", "01:00", "01:10", "01:20"),
("E5001-2", 1, "09:10", "09:20", "09:30", "09:40", "09:50", "10:00", "10:10", "10:20"),
("E5001-3", 1, "18:10", "18:20", "18:30", "18:40", "18:50", "19:00", "19:10", "19:20"),
("E5001-4", 1, "21:10", "21:20", "21:30", "21:40", "21:50", "22:00", "22:10", "22:20"),
("E5002-1", 2, "02:15", "02:25", "02:35", "02:45", "02:55", "03:05", "03:15", "03:25"),
("E5002-2", 2, "11:15", "11:25", "11:35", "11:45", "11:55", "12:05", "12:15", "12:25"),
("E5002-3", 2, "20:15", "20:25", "20:35", "20:45", "20:55", "21:05", "21:15", "21:25"),
("E5003-1", 3, "04:20", "04:30", "04:40", "04:50", "05:00", "05:10", "05:20", "05:30"),
("E5003-2", 3, "13:20", "13:30", "13:40", "13:50", "14:00", "14:10", "14:20", "14:30"),
("E5003-3", 3, "22:20", "22:30", "22:40", "22:50", "23:00", "23:10", "23:20", "23:30"),
("E4001-1", 4, "06:25", "06:35", "06:45", "06:55", "07:05", "07:15", "07:25", "07:35"),
("E4001-2", 4, "15:25", "15:35", "15:45", "15:55", "16:05", "16:15", "16:25", "16:35"),
("E4001-3", 4, "00:25", "00:35", "00:45", "00:55", "01:05", "01:15", "01:25", "01:35"),
("E4002-1", 5, "08:30", "08:40", "08:50", "09:00", "09:10", "09:20", "09:30", "09:40"),
("E4002-2", 5, "17:30", "17:40", "17:50", "18:00", "18:10", "18:20", "18:30", "18:40"),
("E4002-3", 5, "02:30", "02:40", "02:50", "03:00", "03:10", "03:20", "03:30", "03:40"),
("E4003-1", 6, "10:35", "10:45", "10:55", "11:05", "11:15", "11:25", "11:35", "11:45"),
("E4003-2", 6, "19:35", "19:45", "19:55", "20:05", "20:15", "20:25", "20:35", "20:45"),
("B4001-1", 7, "12:40", "12:50", "13:00", "13:10", "13:20", "13:30", "13:40", "13:50"),
("B4001-2", 7, "21:40", "21:50", "22:00", "22:10", "22:20", "22:30", "22:40", "22:50"),
("B4002-1", 8, "14:45", "14:55", "15:05", "15:15", "15:25", "15:35", "15:45", "15:55"),
("B4002-2", 8, "23:45", "23:55", "00:05", "00:15", "00:25", "00:35", "00:45", "00:55"),
("F3001-1", 9, "16:50", "17:00", "17:10", "17:20", "17:30", "17:40", "17:50", "18:00"),
("L2001-1", 10, "18:55", "19:05", "19:15", "19:25", "19:35", "19:45", "19:55", "20:05");

LOCK TABLES `reservations` WRITE;
/*!40000 ALTER TABLE `reservations` DISABLE KEYS */;
INSERT INTO `reservations` VALUES ('01JFCWJY91V74EKGZQKN9HJD23','aea4e2dc-3fb2-42eb-8070-97cf0c129a41','E4002-1','A','B','08:30','01JFCWJY92NVY3P585VXZB6TK9','2024-12-18 12:41:19.906610'),('01JFCWK8256KS2FS9AZFZ4KA4R','aea4e2dc-3fb2-42eb-8070-97cf0c129a41','E4002-1','B','A','08:40','01JFCWK825KKRSP9YEVT23BB2W','2024-12-18 12:41:29.925968'),('01JFCWKK3P9CYZ6VRFHF1HJEMH','aea4e2dc-3fb2-42eb-8070-97cf0c129a41','E4002-2','C','D','17:50','01JFCWKK3P8MW7Z76KS88HATD1','2024-12-18 12:41:41.239044'),('01JFCWKWWND04N491F4AZ5ZPFY','aea4e2dc-3fb2-42eb-8070-97cf0c129a41','E5001-3','D','E','18:40','01JFCWKWWN1FABWQNA1DVWJ8ZJ','2024-12-18 12:41:51.254047');/*!40000 ALTER TABLE `reservations` ENABLE KEYS */;
UNLOCK TABLES;

LOCK TABLES `reservation_seats` WRITE;
/*!40000 ALTER TABLE `reservation_seats` DISABLE KEYS */;
INSERT INTO `reservation_seats` VALUES (1,'01JFCWJY91V74EKGZQKN9HJD23','1-A','2024-12-18 12:41:19.909238'),(2,'01JFCWJY91V74EKGZQKN9HJD23','1-B','2024-12-18 12:41:19.910702'),(3,'01JFCWK8256KS2FS9AZFZ4KA4R','1-C','2024-12-18 12:41:29.928164'),(4,'01JFCWK8256KS2FS9AZFZ4KA4R','1-D','2024-12-18 12:41:29.928635'),(5,'01JFCWKK3P9CYZ6VRFHF1HJEMH','1-A','2024-12-18 12:41:41.240609'),(6,'01JFCWKK3P9CYZ6VRFHF1HJEMH','1-B','2024-12-18 12:41:41.240848'),(7,'01JFCWKK3P9CYZ6VRFHF1HJEMH','1-C','2024-12-18 12:41:41.241120'),(8,'01JFCWKK3P9CYZ6VRFHF1HJEMH','1-D','2024-12-18 12:41:41.241312'),(9,'01JFCWKK3P9CYZ6VRFHF1HJEMH','2-A','2024-12-18 12:41:41.241485'),(10,'01JFCWKK3P9CYZ6VRFHF1HJEMH','2-B','2024-12-18 12:41:41.241662'),(11,'01JFCWKK3P9CYZ6VRFHF1HJEMH','2-C','2024-12-18 12:41:41.241864'),(12,'01JFCWKK3P9CYZ6VRFHF1HJEMH','2-D','2024-12-18 12:41:41.242122'),(13,'01JFCWKK3P9CYZ6VRFHF1HJEMH','3-A','2024-12-18 12:41:41.242281'),(14,'01JFCWKK3P9CYZ6VRFHF1HJEMH','3-B','2024-12-18 12:41:41.242420'),(15,'01JFCWKWWND04N491F4AZ5ZPFY','1-A','2024-12-18 12:41:51.256592'),(16,'01JFCWKWWND04N491F4AZ5ZPFY','1-B','2024-12-18 12:41:51.256987'),(17,'01JFCWKWWND04N491F4AZ5ZPFY','1-C','2024-12-18 12:41:51.257196'),(18,'01JFCWKWWND04N491F4AZ5ZPFY','1-D','2024-12-18 12:41:51.257460'),(19,'01JFCWKWWND04N491F4AZ5ZPFY','1-E','2024-12-18 12:41:51.257964'),(20,'01JFCWKWWND04N491F4AZ5ZPFY','2-A','2024-12-18 12:41:51.258219'),(21,'01JFCWKWWND04N491F4AZ5ZPFY','2-B','2024-12-18 12:41:51.258675'),(22,'01JFCWKWWND04N491F4AZ5ZPFY','2-C','2024-12-18 12:41:51.258904'),(23,'01JFCWKWWND04N491F4AZ5ZPFY','2-D','2024-12-18 12:41:51.259091'),(24,'01JFCWKWWND04N491F4AZ5ZPFY','2-E','2024-12-18 12:41:51.259258');
/*!40000 ALTER TABLE `reservation_seats` ENABLE KEYS */;
UNLOCK TABLES;

LOCK TABLES `payments` WRITE;
/*!40000 ALTER TABLE `payments` DISABLE KEYS */;
INSERT INTO `payments` VALUES (1,'aea4e2dc-3fb2-42eb-8070-97cf0c129a41','01JFCWJY91V74EKGZQKN9HJD23',2000,1,0,'2024-12-18 12:41:19.912851','2024-12-18 12:41:20.971842'),(2,'aea4e2dc-3fb2-42eb-8070-97cf0c129a41','01JFCWK8256KS2FS9AZFZ4KA4R',14000,1,0,'2024-12-18 12:41:29.930964','2024-12-18 12:41:31.015337'),(3,'aea4e2dc-3fb2-42eb-8070-97cf0c129a41','01JFCWKK3P9CYZ6VRFHF1HJEMH',10000,1,0,'2024-12-18 12:41:41.243943','2024-12-18 12:41:44.268173'),(4,'aea4e2dc-3fb2-42eb-8070-97cf0c129a41','01JFCWKWWND04N491F4AZ5ZPFY',10000,1,0,'2024-12-18 12:41:51.260898','2024-12-18 12:41:52.832747');/*!40000 ALTER TABLE `payments` ENABLE KEYS */;
UNLOCK TABLES;

LOCK TABLES `entries` WRITE;
/*!40000 ALTER TABLE `entries` DISABLE KEYS */;
INSERT INTO `entries` VALUES (1,'01JFCWJY91V74EKGZQKN9HJD23','2024-12-18 12:48:15.186353');
/*!40000 ALTER TABLE `entries` ENABLE KEYS */;
UNLOCK TABLES;
