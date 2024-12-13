insert into stations values ("A", "Arena"), ("B", "Bridge"), ("C", "Cave"), ("D", "Dock"), ("E", "Edge");

-- bcrypt.hashpw("admin".encode(), "$2b$12$Lh9gc29tkPwGw0TPwTqjoO".encode()).decode()
-- bcrypt.hashpw("ishocon".encode(), "$2b$12$W2VwGdhCXQt4ef6Zbrxnke".encode()).decode()
insert into users (`id`, `name`, `hashed_password`, salt, is_admin, global_payment_token) values
("72b4f686-d283-4b63-b8bb-1e1fa7529c11", "admin", "$2b$12$Lh9gc29tkPwGw0TPwTqjoORtwJMvoJAXaXmurqNAcveQnSbHXkf8K", "$2b$12$Lh9gc29tkPwGw0TPwTqjoO", 1, "96d230c6-add1-498e-ac58-95212c9e869e"),
("aea4e2dc-3fb2-42eb-8070-97cf0c129a41", "ishocon", "$2b$12$W2VwGdhCXQt4ef6Zbrxnke4VO6PRVD0uZU5GAmVNjMp88CIT6hbI.", "$2b$12$W2VwGdhCXQt4ef6Zbrxnke", 0, "362f462c-1cb8-4054-890a-944c2b2437ef");
