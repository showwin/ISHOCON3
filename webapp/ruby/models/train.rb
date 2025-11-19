# frozen_string_literal: true

class Train < ActiveRecord::Base
  attribute :id, :string
  attribute :name, :string
  attribute :model, :string
end
