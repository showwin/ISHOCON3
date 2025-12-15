# frozen_string_literal: true

require 'English'
require 'sinatra'
require 'sinatra/activerecord'
require 'sinatra/param'
require 'active_support/all'
require 'bcrypt'
require 'ulid'
require 'redis'

require_relative 'config/environment'
require_relative 'middlewares/auth'
require_relative 'services/payment_app'
require_relative 'utils/util'

Dir[File.join(__dir__, 'models', '*.rb')].sort.each { |f| require f }

helpers Sinatra::Param
register Auth

# Redis connection for seat management
REDIS_HOST = ENV.fetch('ISHOCON_REDIS_HOST', 'localhost')
REDIS_PORT = ENV.fetch('ISHOCON_REDIS_PORT', '6379').to_i
REDIS = Redis.new(host: REDIS_HOST, port: REDIS_PORT, db: 0)

WAITING_ROOM_CONFIG = {
  max_active_users: 150,
  polling_interval_ms: 1000
}.freeze

SESSION_CONFIG = {
  idle_timeout_sec: 7,
  polling_interval_ms: 500
}.freeze

before do
  content_type :json
  body = request.body.read
  params.merge!(JSON.parse(body)) unless body.empty?
end

post '/api/initialize' do
  output = `/home/ishocon/webapp/sql/init.sh 2>&1`

  halt 500, "Failed to initialize: #{output}" unless $CHILD_STATUS.success?

  begin
    PaymentApp.initialize
  rescue PaymentApp::PaymentAppInitializationFailed => e
    halt 500, e.message
  end

  Setting.transaction do
    Setting.delete_all
    Setting.create!
  end

  # Initialize Redis with available seats for all schedules
  REDIS.flushdb
  TrainSchedule.find_each do |schedule|
    Util.initialize_schedule_seats(REDIS, schedule)
  end

  {
    initialized_at: Setting.first.initialized_at.utc.strftime('%Y-%m-%dT%H:%M:%S.%6NZ'),
    app_language: 'ruby'
  }.to_json
end

get '/api/current_time' do
  { current_time: Util.application_clock }.to_json
end

get '/api/stations' do
  { stations: Station.all }.to_json
end

get '/api/schedules' do
  current_time = Util.application_clock
  current_hour, current_minute = current_time.split(':').map(&:to_i)

  # Use multiple smart queries for better train distribution
  two_hours_later = format('%02d:%02d', current_hour + 2, current_minute)
  six_hours_later = format('%02d:%02d', (current_hour + 6) % 24, current_minute)
  twelve_hours_later = format('%02d:%02d', (current_hour + 12) % 24, current_minute)

  # Query 1: Trains departing 2+ hours later
  query1 = TrainSchedule
           .where('departure_at_station_a_to_b >= ?', two_hours_later)
           .where.not('id LIKE ?', 'L2%')
           .order(:departure_at_station_a_to_b)
           .limit(4)

  # Query 2: Trains departing 6+ hours later
  query2 = TrainSchedule
           .where('departure_at_station_a_to_b >= ?', six_hours_later)
           .where.not('id LIKE ?', 'L2%')
           .order(:departure_at_station_a_to_b)
           .limit(2)

  # Query 3: Trains departing 12+ hours later
  query3 = TrainSchedule
           .where('departure_at_station_a_to_b >= ?', twelve_hours_later)
           .where.not('id LIKE ?', 'L2%')
           .order(:departure_at_station_a_to_b)
           .limit(2)

  # Query 4: Trains departing between 19:00-22:00
  query4 = TrainSchedule
           .where('departure_at_station_a_to_b > ? AND departure_at_station_a_to_b < ?', '19:00', '22:00')
           .where.not('id LIKE ?', 'L2%')
           .order("rand()")
           .limit(1)

  # Query 5: Trains departing between 22:00-24:00
  query5 = TrainSchedule
           .where('departure_at_station_b_to_a > ?', '22:00')
           .where.not('id LIKE ?', 'L2%')
           .order("rand()")
           .limit(1)

  # Combine all results and remove duplicates
  all_schedules = (query1.to_a + query2.to_a + query3.to_a + query4.to_a + query5.to_a).uniq(&:id).take(10)

  trains = []
  all_schedules.each do |schedule|
    train = Train.find(schedule.train_id)

    # Get available seats from Redis
    redis_key = "schedule:#{schedule.id}:available_seats"
    available_seats = REDIS.llen(redis_key)

    # Get total seats for this train
    total_seats = Train
                  .joins('INNER JOIN train_models ON trains.model = train_models.name')
                  .where(id: train.id)
                  .select('train_models.seat_rows * train_models.seat_columns AS total_seats')
                  .take
                  .total_seats.to_i

    # Use the same availability sign for all station pairs
    availability_sign = Util.available_seat_sign(available_seats, total_seats)

    trains << {
      'id' => schedule.id,
      'availability' => {
        'Arena->Bridge' => availability_sign,
        'Bridge->Cave' => availability_sign,
        'Cave->Dock' => availability_sign,
        'Dock->Edge' => availability_sign,
        'Edge->Dock' => availability_sign,
        'Dock->Cave' => availability_sign,
        'Cave->Bridge' => availability_sign,
        'Bridge->Arena' => availability_sign
      },
      'departure_at' => {
        'Arena->Bridge' => schedule.departure_at_station_a_to_b,
        'Bridge->Cave' => schedule.departure_at_station_b_to_c,
        'Cave->Dock' => schedule.departure_at_station_c_to_d,
        'Dock->Edge' => schedule.departure_at_station_d_to_e,
        'Edge->Dock' => schedule.departure_at_station_e_to_d,
        'Dock->Cave' => schedule.departure_at_station_d_to_c,
        'Cave->Bridge' => schedule.departure_at_station_c_to_b,
        'Bridge->Arena' => schedule.departure_at_station_b_to_a
      }
    }
  end

  { schedules: trains }.to_json
end

get '/api/purchased_tickets', user_auth: true do
  # JA: このAPIは予約ページのリロード時に呼ばれるので、ユーザはアクティブだと判断してユーザーの最終アクティビティを更新する
  # EN: This API is called when the reservation page is reloaded, so it determines that the user is active and updates the user's last activity.
  Util.update_last_activity_at(@current_user.id)

  tickets = []

  reservations = Reservation
                 .joins('INNER JOIN payments p ON p.reservation_id = reservations.id')
                 .joins('INNER JOIN reservation_qr_images qr ON qr.reservation_id = reservations.id')
                 .joins('INNER JOIN stations s1 ON reservations.from_station_id = s1.id')
                 .joins('INNER JOIN stations s2 ON reservations.to_station_id = s2.id')
                 .joins('LEFT OUTER JOIN entries e ON e.reservation_id = reservations.id')
                 .where(user_id: @current_user.id)
                 .where('p.is_captured = TRUE')
                 .select(
                   'reservations.id AS reservation_id, ' \
                   'reservations.schedule_id AS schedule_id, ' \
                   's1.name AS from_station, ' \
                   's2.name AS to_station, ' \
                   'reservations.departure_at AS departure_at, ' \
                   'p.amount AS total_price, ' \
                   'reservations.entry_token AS entry_token, ' \
                   'qr.id AS qr_id, ' \
                   'CASE WHEN e.id IS NOT NULL THEN 1 ELSE 0 END AS is_entered'
                 )

  reservations.each do |reservation|
    seats = ReservationSeat
            .where(reservation_id: reservation.reservation_id)
            .pluck(:seat)

    ticket = {
      reservation_id: reservation.reservation_id,
      schedule_id: reservation.schedule_id,
      from_station: reservation.from_station,
      to_station: reservation.to_station,
      departure_at: reservation.departure_at,
      seats: seats,
      total_price: reservation.total_price,
      entry_token: reservation.entry_token,
      qr_code_url: "/api/qr/#{reservation.qr_id}.png",
      is_entered: reservation.is_entered == 1
    }

    tickets << ticket
  end

  { tickets: tickets }.to_json
end

post '/api/reserve', user_auth: true do
  param :schedule_id, String, required: true
  param :from_station_id, String, required: true
  param :to_station_id, String, required: true
  param :num_people, Integer, required: true

  Util.update_last_activity_at(@current_user.id)

  # Use Redis-based atomic seat picking (no locks needed)
  reserved_schedule_id, seats = Util.pick_seats(
    REDIS,
    params[:schedule_id],
    params[:from_station_id],
    params[:to_station_id],
    params[:num_people]
  )

  if reserved_schedule_id.nil?
    return {
      status: 'fail',
      error_code: 'NO_SEAT_AVAILABLE'
    }.to_json
  end

  departure_at = Util.departure_at(
    reserved_schedule_id,
    params[:from_station_id],
    params[:to_station_id]
  )

  reservation_id = ULID.generate
  entry_token = ULID.generate

  reservation = Reservation.create!(
    id: reservation_id,
    user_id: @current_user.id,
    schedule_id: reserved_schedule_id,
    from_station_id: params[:from_station_id],
    to_station_id: params[:to_station_id],
    departure_at: departure_at,
    entry_token: entry_token
  )

  seats.each do |seat|
    ReservationSeat.create!(
      reservation_id: reservation.id,
      seat: seat
    )
  end

  total_price, is_discounted = Util.calculate_seat_price(reservation, seats)
  Payment.create!(
    user_id: @current_user.id,
    reservation_id: reservation.id,
    amount: total_price
  )

  from_station = Station.find(reservation.from_station_id)
  to_station   = Station.find(reservation.to_station_id)

  status = if reservation.schedule_id == params[:schedule_id]
             'success'
           else
             'recommend'
           end

  reservation = {
    reservation_id: reservation.id,
    schedule_id: reservation.schedule_id,
    from_station: from_station.name,
    to_station: to_station.name,
    departure_at: reservation.departure_at,
    seats: seats,
    total_price: total_price,
    is_discounted: is_discounted
  }

  response = { status: status }
  response[:reserved] = reservation if status == 'success'
  response[:recommend] = reservation if status == 'recommend'

  response.to_json
end

post '/api/purchase', user_auth: true do
  param :reservation_id, String, required: true

  Util.update_last_activity_at(@current_user.id)

  reservation = Reservation.find(params[:reservation_id])

  halt 401, { message: 'Invalid reservation' }.to_json if reservation.user_id != @current_user.id

  payment = Payment.find_by!(reservation_id: reservation.id)

  resp = PaymentApp.capture_payment(payment.amount, @current_user.global_payment_token)
  resp_body = JSON.parse(resp.body)
  payment_status = resp_body['status'] == 'accepted' ? 'success' : 'failed'
  resp_message   = resp_body['message']

  if payment_status == 'success'
    payment.update(is_captured: true)
  else
    # Return seats to Redis on payment failure
    Util.release_seat_reservation(REDIS, reservation)
  end

  qr_image = Util.generate_qr_image(reservation.entry_token)
  image_id = ULID.generate

  ReservationQrImage.create!(
    id: image_id,
    reservation_id: reservation.id,
    image: qr_image
  )

  {
    status: payment_status,
    message: resp_message,
    entry_token: payment_status == 'success' ? reservation.entry_token : '',
    qr_code_url: payment_status == 'success' ? "/api/qr/#{image_id}.png" : ''
  }.to_json
end

get '/api/qr/:qr_id.png' do
  content_type 'image/png'

  qr_id = params[:qr_id]

  qr_image = ReservationQrImage.find_by(id: qr_id)

  halt 404 unless qr_image

  qr_image.image
end

post '/api/entry' do
  param :entry_token, String, required: true

  reservation = Reservation.find_by(entry_token: params[:entry_token])

  halt 404, { message: "Invalid entry token: #{params[:entry_token]}" }.to_json if reservation.nil?

  # JA: 列車の発車時間を過ぎていないことを確認
  # EN: Confirm that the train has not departed yet
  return { status: 'train_departed' }.to_json if reservation.departure_at < Util.application_clock

  Entry.create!(reservation_id: reservation.id)

  { status: 'success' }.to_json
end

post '/api/refund', user_auth: true do
  param :reservation_id, String, required: true

  Util.update_last_activity_at(@current_user.id)

  reservation = Reservation.find_by(id: params[:reservation_id])

  if reservation.nil? || reservation.user_id != @current_user.id
    return {
      status: 'fail',
      error_code: 'INVALID_RESERVATION'
    }.to_json
  end

  payment = Payment.find_by(reservation_id: reservation.id)

  if payment.nil? || !payment.is_captured
    return {
      status: 'fail',
      error_code: 'NOT_CAPTURED'
    }.to_json
  end

  entry = Entry.find_by(reservation_id: reservation.id)
  if entry
    return {
      status: 'fail',
      error_code: 'ALREADY_ENTERED'
    }.to_json
  end

  payment.update(is_captured: false, is_refunded: true)

  # Return seats to Redis if train hasn't departed yet
  Util.release_seat_reservation(REDIS, reservation) if reservation.departure_at > Util.application_clock

  { status: 'success' }.to_json
end

get '/api/session', user_auth: true do
  # JA: 開発中に短時間でセッションが切れるのは不便なので、ishoconユーザはセッションを切れないようにしておくと便利
  # EN: During development, it is inconvenient for the session to expire in a short time,
  if @current_user.name == 'ishocon'
    return {
      status: 'active',
      next_check: 9_999_999_999
    }.to_json
  end

  # JA: `idle_timeout_sec` 秒以上アクティブでないユーザはログアウトさせる
  # EN: Log out users who have been inactive for more than `idle_timeout_sec` seconds
  if @current_user.last_activity_at && @current_user.last_activity_at < (Time.now - SESSION_CONFIG[:idle_timeout_sec])
    response.delete_cookie('user_name')

    return {
      status: 'session_expired',
      next_check: SESSION_CONFIG[:polling_interval_ms]
    }.to_json
  end

  {
    status: 'active',
    next_check: SESSION_CONFIG[:polling_interval_ms]
  }.to_json
end

def handle_login
  param :name,     String, required: true
  param :password, String, required: true

  user = User.find_by(name: params[:name])

  halt 401, { message: 'Invalid name or password' }.to_json if user.nil?

  hashed_password = BCrypt::Engine.hash_secret(params[:password], user.salt)

  halt 401, { message: 'Invalid name or password' }.to_json unless user.hashed_password == hashed_password

  if user.is_admin
    response.set_cookie('admin_name', value: user.name, httponly: true)
  else
    response.set_cookie('user_name', value: user.name, httponly: true)
  end

  Util.update_last_activity_at(user.id)

  {
    status: 'success',
    user: {
      id: user.id,
      name: user.name,
      is_admin: user.is_admin
    }
  }.to_json
end

post '/api/login' do
  handle_login
end

post '/api/admin/login' do
  handle_login
end

post '/api/logout' do
  response.delete_cookie('user_name')
  response.delete_cookie('admin_name')

  { status: 'success' }.to_json
end

## Waiting Room

get '/api/waiting_status', user_auth: true do
  active_user_count = User
                      .where('last_activity_at >= ?', Time.now - SESSION_CONFIG[:idle_timeout_sec])
                      .count

  if active_user_count >= WAITING_ROOM_CONFIG[:max_active_users]
    status = 'waiting'
  else
    Util.update_last_activity_at(@current_user.id)
    status = 'ready'
  end

  { status: status, next_check: WAITING_ROOM_CONFIG[:polling_interval_ms] }.to_json
end

## Admin API

get '/api/admin/stats', admin_auth: true do
  total_sales = Payment
                .joins('INNER JOIN entries ON payments.reservation_id = entries.reservation_id')
                .where(is_captured: true)
                .sum(:amount)
                .to_i

  total_refunds = Payment
                  .where(is_refunded: true)
                  .sum(:amount)
                  .to_i

  { total_sales: total_sales, total_refunds: total_refunds }.to_json
end

get '/api/admin/train_sales', admin_auth: true do
  trains = Train
           .joins('INNER JOIN train_schedules s ON trains.id = s.train_id')
           .joins('INNER JOIN reservations r ON s.id = r.schedule_id')
           .joins('INNER JOIN payments p ON r.id = p.reservation_id')
           .joins('LEFT OUTER JOIN entries e ON r.id = e.reservation_id')
           .joins(<<~SQL)
             LEFT JOIN (
               SELECT reservation_id, COUNT(*) AS ticket_count
               FROM reservation_seats
               GROUP BY reservation_id
             ) ticket_counts
             ON r.id = ticket_counts.reservation_id AND p.is_captured = 1
           SQL
           .select(
             'trains.name AS train_name, ' \
             'COALESCE(SUM(ticket_counts.ticket_count), 0) AS tickets_sold, ' \
             'COALESCE(SUM(CASE WHEN e.id IS NULL AND p.is_captured THEN p.amount ELSE 0 END), 0) AS pending_revenue, ' \
             'COALESCE(SUM(CASE WHEN e.id IS NOT NULL AND p.is_captured THEN p.amount ELSE 0 END), 0) AS confirmed_revenue, ' \
             'COALESCE(SUM(CASE WHEN p.is_refunded THEN p.amount ELSE 0 END), 0) AS refunds'
           )
           .group('trains.id, trains.name')

  data = trains.map do |t|
    {
      train_name: t.train_name,
      tickets_sold: t.tickets_sold.to_i,
      pending_revenue: t.pending_revenue.to_i,
      confirmed_revenue: t.confirmed_revenue.to_i,
      refunds: t.refunds.to_i
    }
  end

  { trains: data }.to_json
end

get '/api/train_models' do
  train_models = TrainModel.pluck(:name)

  { model_names: train_models }.to_json
end

post '/api/admin/add_train', admin_auth: true do
  param :train_name,      String, required: true
  param :model_name,      String, required: true
  param :departure_times, Array,  required: true

  train_model = TrainModel.find_by(name: params[:model_name])
  halt 400 if train_model.nil?

  train = Train.create!(
    name: params[:train_name],
    model: params[:model_name]
  )

  params[:departure_times].each_with_index do |departure_time_at_a, i|
    TrainSchedule.create!(
      id: "#{train.name}-#{i + 1}",
      train_id: train.id,
      departure_at_station_a_to_b: departure_time_at_a,
      departure_at_station_b_to_c: Util.add_time(departure_time_at_a, 10),
      departure_at_station_c_to_d: Util.add_time(departure_time_at_a, 20),
      departure_at_station_d_to_e: Util.add_time(departure_time_at_a, 30),
      departure_at_station_e_to_d: Util.add_time(departure_time_at_a, 40),
      departure_at_station_d_to_c: Util.add_time(departure_time_at_a, 50),
      departure_at_station_c_to_b: Util.add_time(departure_time_at_a, 60),
      departure_at_station_b_to_a: Util.add_time(departure_time_at_a, 70)
    )
  end

  schedules = TrainSchedule.where(train_id: train.id)

  schedules.each do |schedule|
    Util.initialize_schedule_seats(REDIS, schedule)
  end

  { status: 'success' }.to_json
end
