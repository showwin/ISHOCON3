# frozen_string_literal: true

require_relative '../models/user'

module Auth
  def require_user!
    user_name = request.cookies['user_name']

    halt 401, { detail: 'user_name cookie is required' }.to_json unless user_name && !user_name.empty?

    user = User.find_by(name: user_name)

    halt 401, { detail: 'Invalid user name' }.to_json unless user

    @current_user = user
  end

  def require_admin!
    admin_name = request.cookies['admin_name']

    halt 401, { detail: 'admin_name cookie is required' }.to_json unless admin_name && !admin_name.empty?

    user = User.find_by(name: admin_name, is_admin: 1)

    halt 401, { detail: 'Invalid admin name' }.to_json unless user

    @current_admin = user
  end

  def self.registered(app)
    app.helpers Auth

    app.set :user_auth do |required|
      app.condition do
        require_user! if required
      end
    end

    app.set :admin_auth do |required|
      app.condition do
        require_admin! if required
      end
    end
  end
end
