# frozen_string_literal: true

require 'rqrcode'
require 'chunky_png'
require 'stringio'

class SeatRowStatus
  attr_accessor :seat_row, :a, :b, :c, :d, :e

  def initialize(record)
    @seat_row = record.seat_row
    @a = record.a >= 1
    @b = record.b >= 1
    @c = record.c >= 1
    @d = record.d >= 1
    @e = record.e >= 1
  end
end

module Util
  module_function

  BASE_TICKET_PRICE = 1000

  module AvailableSeats
    LOTS = 'lots'
    FEW  = 'few'
    NONE = 'none'
  end

  def application_clock
    setting = Setting.first

    seconds_passed = (Time.now - setting.initialized_at).to_f

    days_passed = (seconds_passed / 86_400).floor
    seconds_passed = (seconds_passed - days_passed * 86_400).floor
    micros_passed = ((seconds_passed - days_passed * 86_400 - seconds_passed) * 1_000_000).to_i

    # JA: この世界では1秒が10分に相当する。24:00で世界が止まる。
    # EN: In this world, 1 second corresponds to 10 minutes. The world stops at 24:00.
    hours = [seconds_passed / 6, 24].min

    minutes = if hours < 24
                (((seconds_passed + micros_passed / 1_000_000.0) % 6) * 10).to_i
              else
                0
              end

    format('%02d:%02d', hours, minutes)
  end

  def available_seat_sign(available_seats, total_seats)
    return AvailableSeats::NONE if available_seats.zero?

    return AvailableSeats::FEW if available_seats.to_f / total_seats <= 0.1

    AvailableSeats::LOTS
  end

  def take_lock(schedule_id)
    retry_count = 10
    i = 0

    loop do
      ReservationLock.create!(schedule_id: schedule_id)
    rescue ActiveRecord::RecordNotUnique
      if i >= retry_count
        puts "Failed to take a lock #{schedule_id} after #{retry_count} retries"
        return false
      end

      i += 1
      sleep 0.1
    else
      break
    end

    true
  end

  def release_lock(schedule_id)
    ReservationLock.where(schedule_id: schedule_id).delete_all
  end

  def pick_seats(schedule_id, from_station_id, to_station_id, num_people)
    # JA: 乗車区間を考えるのは大変なので、最初から最後まで全部空いているかどうかだけを考える
    # 本当は乗車区間だけステータスを更新したい…
    # EN: Considering the boarding section is difficult, so we only consider whether it is completely empty from the beginning to the end.
    # Ideally, we would like to update the status only for the boarding section...

    # JA: 全区間空いている席が num_people 以上あるかどうかを確認する
    # EN: Check if there are at least num_people seats available for the entire section
    sql_available_seats = <<~SQL
      SELECT SUM(a + b + c + d + e) AS total_available_seats
      FROM (
        SELECT seat_row,
               MIN(a_is_available) AS a,
               MIN(b_is_available) AS b,
               MIN(c_is_available) AS c,
               MIN(d_is_available) AS d,
               MIN(e_is_available) AS e
        FROM seat_row_reservations
        WHERE schedule_id = ?
        GROUP BY seat_row
      ) AS available_seats;
    SQL
    sql = ActiveRecord::Base.send(:sanitize_sql_array, [sql_available_seats, schedule_id])
    available_seats = ActiveRecord::Base.connection.exec_query(sql).first['total_available_seats'].to_i

    if available_seats < num_people
      # FIXME:
      # JA: 空席が足りない場合は他のスケジュールをレコメンドしたいが、良いアルゴリズムが思い浮かばないので必要になったら実装する。
      # EN: When there are not enough available seats, we want to recommend other schedules, but we can't think of a good algorithm, so we will implement it when necessary.
      return [nil, []]
    end

    seat_rows = SeatRowReservation
                .where(schedule_id: schedule_id)
                .select(
                  'seat_row, ' \
                  'MIN(a_is_available) AS a, ' \
                  'MIN(b_is_available) AS b, ' \
                  'MIN(c_is_available) AS c, ' \
                  'MIN(d_is_available) AS d, ' \
                  'MIN(e_is_available) AS e'
                )
                .group(:seat_row)
                .map { |seat_row| SeatRowStatus.new(seat_row) }

    reserved_seats = []

    num_people.times do
      seat_rows.each do |seat_row|
        if seat_row.a
          reserved_seats << "#{seat_row.seat_row}-A"
          seat_row.a = false
          break
        end

        if seat_row.b
          reserved_seats << "#{seat_row.seat_row}-B"
          seat_row.b = false
          break
        end

        if seat_row.c
          reserved_seats << "#{seat_row.seat_row}-C"
          seat_row.c = false
          break
        end

        if seat_row.d
          reserved_seats << "#{seat_row.seat_row}-D"
          seat_row.d = false
          break
        end

        next unless seat_row.e

        reserved_seats << "#{seat_row.seat_row}-E"
        seat_row.e = false
        break
      end
    end

    # JA: 予約状況を反映
    # EN: Reflect the reservation status
    reserved_seats.each do |seat|
      seat_row, column = seat.split('-')

      SeatRowReservation
        .where(schedule_id: schedule_id, seat_row: seat_row)
        .update_all("#{column.downcase}_is_available = 0")
    end

    [schedule_id, reserved_seats]
  end

  def stations_between(start_id, end_id)
    stations = %w[A B C D E Dr Cr Br Ar]
    station_ids = %w[A B C D E D C B A]

    end_id += 'r' if start_id > end_id

    start_index = stations.index(start_id)
    end_index = stations[start_index..].index(end_id) + start_index

    station_ids[start_index..end_index]
  end

  def calculate_distance(start_id, end_id)
    stations = %w[A B C D E Dr Cr Br Ar]

    end_id += 'r' if start_id > end_id

    start_index = stations.index(start_id)
    end_index = stations[start_index..].index(end_id) + start_index

    end_index - start_index
  end

  def calculate_seat_price(reservation, seats)
    distance = calculate_distance(reservation.from_station_id, reservation.to_station_id)
    num_seats = seats.length

    return [BASE_TICKET_PRICE * distance, false] if num_seats == 1

    train_seat_columns = TrainModel
                         .joins('INNER JOIN trains t ON t.model = train_models.name')
                         .joins('INNER JOIN train_schedules ts ON ts.train_id = t.id')
                         .joins('INNER JOIN reservations r ON r.schedule_id = ts.id')
                         .where('r.id = ?', reservation.id)
                         .select('seat_columns')
                         .take
                         .seat_columns

    allowed_groups = (num_seats.to_f / train_seat_columns).ceil
    seats = seats.sort
    full_price = BASE_TICKET_PRICE * distance * num_seats
    discounted_price = (full_price * 0.5).to_i

    # JA: 必要以上に席が違う列に分かれてしまっている場合は割引料金
    # EN: If seats are divided into more columns than necessary, a discount applies
    seat_rows = seats.map { |s| s.split('-')[0] }.uniq.length

    if seat_rows > allowed_groups
      puts "more than allowed groups. #{seat_rows} > #{allowed_groups} = #{num_seats} / #{train_seat_columns}."
      return [discounted_price, true]
    end

    seat_column_list = %w[A B C D E]
    previous_seat = nil

    seats.each do |seat|
      if previous_seat.nil?
        previous_seat = seat
        next
      end

      previous_row, previous_column = previous_seat.split('-')
      row, column = seat.split('-')

      if row == previous_row
        expected_column = seat_column_list[seat_column_list.index(previous_column) + 1]

        if column == expected_column
          previous_seat = seat
          next
        else
          puts 'not next to each other'

          # JA: 同じ列だが席が隣り合っていない場合は割引料金
          # EN: If seats are in the same row but not adjacent, a discount applies
          return [discounted_price, true]
        end
      end

      previous_seat = seat
    end

    [full_price, false]
  end

  def departure_at(schedule_id, from_station_id, to_station_id)
    stations = stations_between(from_station_id, to_station_id)
    next_station = stations[1]

    schedule = TrainSchedule.find_by(id: schedule_id)
    schedule.send("departure_at_station_#{from_station_id.downcase}_to_#{next_station.downcase}")
  end

  def release_seat_reservation(reservation)
    seats = ReservationSeat
            .where(reservation_id: reservation.id)
            .pluck(:seat)

    # JA: 今の実装では乗車区間は気にせずに全区間に対して席を取得しているので、全区間に対して席を解放する
    # EN: In the current implementation, seats are acquired for the entire section without considering the boarding section, so seats are released for the entire section
    seats.each do |seat|
      seat_row, column = seat.split('-')

      SeatRowReservation
        .where(schedule_id: reservation.schedule_id, seat_row: seat_row)
        .update_all("#{column}_is_available = 1")
    end
  end

  def generate_qr_image(entry_token)
    qr = RQRCode::QRCode.new(entry_token, level: :h)

    png = qr.as_png(
      size: 100,
      border_modules: 4,
      color_mode: ChunkyPNG::COLOR_GRAYSCALE,
      color: 'black',
      fill: 'white'
    )

    io = StringIO.new
    io.write(png.to_s)
    io.string
  end

  def add_time(time_str, minutes)
    h, m = time_str.split(':').map(&:to_i)
    h += (m + minutes) / 60
    m = (m + minutes) % 60

    format('%02d:%02d', h, m)
  end

  def update_last_activity_at(user_id)
    User.find(user_id).update(last_activity_at: DateTime.now)
  end
end
